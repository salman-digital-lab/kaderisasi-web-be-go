// Package google exchanges a Google OAuth authorization code (PKCE) and
// verifies the returned ID token the way web-be's google_identity_service.ts
// does with google-auth-library's OAuth2Client.getToken and verifyIdToken.
package google

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/golang-jwt/jwt/v5"

	"kaderisasi/web/internal/vine"
)

const (
	DefaultTokenURL = "https://oauth2.googleapis.com/token"
	DefaultCertsURL = "https://www.googleapis.com/oauth2/v1/certs"
	// google-auth-library's verifyIdToken defaults.
	clockSkew = 300 * time.Second
	maxExpiry = 86400 * time.Second
)

// LoginError carries the message web-be returns for a refused identity.
type LoginError struct{ Code string }

func (e *LoginError) Error() string { return e.Code }

func failed() error { return &LoginError{"GOOGLE_LOGIN_FAILED"} }

type Client struct {
	ClientID, ClientSecret, RedirectURI string
	TokenURL, CertsURL                  string
	HTTP                                *http.Client
	Now                                 func() time.Time

	mu      sync.Mutex
	certs   map[string]*rsa.PublicKey
	expires time.Time
}

// Identity is readGoogleIdentity's result.
type Identity struct{ Email, Name string }

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// VerifyCode is verifyGoogleCode.
func (c *Client) VerifyCode(ctx context.Context, code, codeVerifier, nonce string) (Identity, error) {
	return c.VerifyCodeFor(ctx, code, codeVerifier, nonce, c.RedirectURI)
}

// VerifyCodeFor exchanges a code obtained with another registered redirect
// URI (for example the Perintis app); callers check it against an allowlist.
func (c *Client) VerifyCodeFor(ctx context.Context, code, codeVerifier, nonce, redirectURI string) (Identity, error) {
	idToken, err := c.exchange(ctx, code, codeVerifier, redirectURI)
	if err != nil {
		return Identity{}, err
	}
	claims, err := c.verify(ctx, idToken)
	if err != nil {
		return Identity{}, err
	}
	return ReadIdentity(claims, nonce)
}

func (c *Client) exchange(ctx context.Context, code, codeVerifier, redirectURI string) (string, error) {
	tokenURL := c.TokenURL
	if tokenURL == "" {
		tokenURL = DefaultTokenURL
	}
	form := url.Values{"code": {code}, "client_id": {c.ClientID}, "client_secret": {c.ClientSecret}, "redirect_uri": {redirectURI},
		"grant_type": {"authorization_code"}, "code_verifier": {codeVerifier}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.http().Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("google token endpoint returned %d", response.StatusCode)
	}
	var tokens struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &tokens); err != nil {
		return "", err
	}
	if tokens.IDToken == "" {
		return "", failed()
	}
	return tokens.IDToken, nil
}

var maxAge = regexp.MustCompile(`max-age=(\d+)`)

func (c *Client) keys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.certs != nil && c.now().Before(c.expires) {
		return c.certs, nil
	}
	certsURL := c.CertsURL
	if certsURL == "" {
		certsURL = DefaultCertsURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, certsURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.http().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google certificates returned %d", response.StatusCode)
	}
	var pems map[string]string
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&pems); err != nil {
		return nil, err
	}
	certs := map[string]*rsa.PublicKey{}
	for id, value := range pems {
		block, _ := pem.Decode([]byte(value))
		if block == nil {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		if key, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			certs[id] = key
		}
	}
	c.certs, c.expires = certs, c.now()
	if m := maxAge.FindStringSubmatch(response.Header.Get("Cache-Control")); m != nil {
		seconds, _ := strconv.Atoi(m[1])
		c.expires = c.now().Add(time.Duration(seconds) * time.Second)
	}
	return certs, nil
}

func (c *Client) verify(ctx context.Context, idToken string) (jwt.MapClaims, error) {
	certs, err := c.keys(ctx)
	if err != nil {
		return nil, err
	}
	claims := jwt.MapClaims{}
	_, err = jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}), jwt.WithoutClaimsValidation()).ParseWithClaims(idToken, claims, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		if key, ok := certs[kid]; ok {
			return key, nil
		}
		return nil, errors.New("no pem found for envelope")
	})
	if err != nil {
		return nil, err
	}
	iat, okIat := claims["iat"].(float64)
	exp, okExp := claims["exp"].(float64)
	if !okIat || !okExp {
		return nil, errors.New("token has no issue or expiration time")
	}
	now := c.now()
	issued, expires := time.Unix(int64(iat), 0), time.Unix(int64(exp), 0)
	if expires.Sub(now) > maxExpiry || now.Before(issued.Add(-clockSkew)) || now.After(expires.Add(clockSkew)) {
		return nil, errors.New("token used outside its validity window")
	}
	if iss, _ := claims["iss"].(string); iss != "accounts.google.com" && iss != "https://accounts.google.com" {
		return nil, errors.New("invalid issuer")
	}
	if aud, _ := claims["aud"].(string); aud != c.ClientID {
		return nil, errors.New("wrong recipient")
	}
	return claims, nil
}

func truthy(v any) bool {
	switch value := v.(type) {
	case nil:
		return false
	case bool:
		return value
	case string:
		return value != ""
	case float64:
		return value != 0
	}
	return true
}

// ReadIdentity is readGoogleIdentity.
func ReadIdentity(claims map[string]any, nonce string) (Identity, error) {
	email, _ := claims["email"].(string)
	if !truthy(claims["sub"]) || email == "" || !truthy(claims["email_verified"]) || claims["nonce"] != nonce {
		return Identity{}, failed()
	}
	email = strings.ToLower(vine.TrimJS(email))
	if !strings.HasSuffix(email, "@gmail.com") && !truthy(claims["hd"]) {
		return Identity{}, &LoginError{"GOOGLE_EMAIL_PASSWORD_REQUIRED"}
	}
	name := ""
	if n, ok := claims["name"].(string); ok {
		units := utf16.Encode([]rune(vine.TrimJS(n)))
		if len(units) > 255 {
			units = units[:255]
		}
		name = string(utf16.Decode(units))
	}
	if name == "" {
		name = strings.SplitN(email, "@", 2)[0]
	}
	return Identity{Email: email, Name: name}, nil
}
