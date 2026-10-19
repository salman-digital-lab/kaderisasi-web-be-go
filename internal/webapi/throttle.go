package webapi

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// clientIP is Adonis's request.ip() with the default trustProxy
// (proxy-addr trusting loopback): walk from the socket address back through
// X-Forwarded-For while the hop is a loopback address.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addresses := []string{host}
	if forwarded := r.Header.Values("X-Forwarded-For"); len(forwarded) > 0 {
		parts := strings.Split(strings.Join(forwarded, ","), ",")
		for i := len(parts) - 1; i >= 0; i-- {
			if p := strings.TrimSpace(parts[i]); p != "" {
				addresses = append(addresses, p)
			}
		}
	}
	for _, a := range addresses {
		if ip := net.ParseIP(a); ip == nil || !ip.IsLoopback() {
			return a
		}
	}
	return addresses[len(addresses)-1]
}

type rateBucket struct {
	count   int
	resetAt int64 // milliseconds
}

// certificateThrottle is public_certificate_throttle_middleware.ts: one
// process-local bucket per client IP shared by both public certificate
// routes, at most 10 000 buckets (expired ones are evicted first, then the
// oldest inserted).
type certificateThrottle struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
	order   []string // insertion order, like a JavaScript Map
	limit   int
	window  time.Duration
	now     func() time.Time
}

const maxBuckets = 10_000

func newCertificateThrottle(now func() time.Time) *certificateThrottle {
	return &certificateThrottle{buckets: map[string]*rateBucket{}, limit: 30, window: time.Minute, now: now}
}

func (t *certificateThrottle) removeExpired(now int64) {
	kept := t.order[:0]
	for _, key := range t.order {
		if t.buckets[key].resetAt <= now {
			delete(t.buckets, key)
		} else {
			kept = append(kept, key)
		}
	}
	t.order = kept
}

func (t *certificateThrottle) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.mu.Lock()
		now := t.now().UnixMilli()
		if len(t.buckets) >= maxBuckets {
			t.removeExpired(now)
			if len(t.buckets) >= maxBuckets {
				delete(t.buckets, t.order[0])
				t.order = t.order[1:]
			}
		}
		key := clientIP(r)
		bucket, ok := t.buckets[key]
		if !ok {
			t.order = append(t.order, key)
		}
		if !ok || bucket.resetAt <= now {
			bucket = &rateBucket{resetAt: now + t.window.Milliseconds()}
			t.buckets[key] = bucket
		}
		bucket.count++
		count, resetAt := bucket.count, bucket.resetAt
		t.mu.Unlock()

		remaining := t.limit - count
		if remaining < 0 {
			remaining = 0
		}
		retryAfter := int64(math.Ceil(float64(resetAt-now) / 1000))
		if retryAfter < 1 {
			retryAfter = 1
		}
		h := w.Header()
		h.Set("RateLimit-Limit", strconv.Itoa(t.limit))
		h.Set("RateLimit-Remaining", strconv.Itoa(remaining))
		h.Set("RateLimit-Reset", strconv.FormatInt(int64(math.Ceil(float64(resetAt)/1000)), 10))
		if count > t.limit {
			h.Set("Retry-After", strconv.FormatInt(retryAfter, 10))
			h.Set("Cache-Control", "no-store, private")
			writeJSON(w, http.StatusTooManyRequests, message{"TOO_MANY_REQUESTS"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
