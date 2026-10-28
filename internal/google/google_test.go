package google

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const clientID = "contract-client.apps.googleusercontent.com"

var now = time.Unix(1800000000, 0)

type fakeGoogle struct {
	key     *rsa.PrivateKey
	server  *httptest.Server
	idToken string
	form    map[string]string
	certs   int
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fake"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeGoogle{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/certs", func(w http.ResponseWriter, _ *http.Request) {
		f.certs++
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_ = json.NewEncoder(w).Encode(map[string]string{"kid-1": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.form = map[string]string{}
		for k := range r.PostForm {
			f.form[k] = r.PostForm.Get(k)
		}
		if r.PostForm.Get("code") == "bad" {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id_token": f.idToken, "access_token": "x"})
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeGoogle) sign(claims jwt.MapClaims, kid string) string {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, _ := token.SignedString(f.key)
	return signed
}

func (f *fakeGoogle) client() *Client {
	return &Client{ClientID: clientID, ClientSecret: "secret", RedirectURI: "http://web.test/callback", TokenURL: f.server.URL + "/token", CertsURL: f.server.URL + "/certs", Now: func() time.Time { return now }}
}

func claims(overrides jwt.MapClaims) jwt.MapClaims {
	c := jwt.MapClaims{"iss": "https://accounts.google.com", "aud": clientID, "sub": "123", "email": " Member@Gmail.com ", "email_verified": true,
		"nonce": strings.Repeat("n", 32), "name": "  Member Name  ", "iat": now.Unix() - 10, "exp": now.Unix() + 3600}
	for k, v := range overrides {
		if v == nil {
			delete(c, k)
		} else {
			c[k] = v
		}
	}
	return c
}

func TestVerifyCode(t *testing.T) {
	f := newFakeGoogle(t)
	nonce := strings.Repeat("n", 32)
	c := f.client()
	f.idToken = f.sign(claims(nil), "kid-1")
	identity, err := c.VerifyCode(context.Background(), "the-code", strings.Repeat("v", 43), nonce)
	if err != nil || identity != (Identity{Email: "member@gmail.com", Name: "Member Name"}) {
		t.Fatalf("identity %+v %v", identity, err)
	}
	want := map[string]string{"code": "the-code", "code_verifier": strings.Repeat("v", 43), "client_id": clientID, "client_secret": "secret", "redirect_uri": "http://web.test/callback", "grant_type": "authorization_code"}
	for k, v := range want {
		if f.form[k] != v {
			t.Errorf("token request %s = %q, want %q", k, f.form[k], v)
		}
	}
	if _, err := c.VerifyCode(context.Background(), "the-code", "v", nonce); err != nil || f.certs != 1 {
		t.Fatalf("certificates not cached: %d fetches, %v", f.certs, err)
	}

	refusals := map[string]struct {
		token string
		code  string
		want  string // "" means any non-LoginError failure
	}{
		"exchange rejected":    {f.sign(claims(nil), "kid-1"), "bad", ""},
		"unknown key":          {f.sign(claims(nil), "kid-2"), "c", ""},
		"wrong audience":       {f.sign(claims(jwt.MapClaims{"aud": "other"}), "kid-1"), "c", ""},
		"wrong issuer":         {f.sign(claims(jwt.MapClaims{"iss": "https://evil.example"}), "kid-1"), "c", ""},
		"expired":              {f.sign(claims(jwt.MapClaims{"exp": now.Unix() - 301, "iat": now.Unix() - 3600}), "kid-1"), "c", ""},
		"lifetime too long":    {f.sign(claims(jwt.MapClaims{"exp": now.Unix() + 90000}), "kid-1"), "c", ""},
		"missing iat":          {f.sign(claims(jwt.MapClaims{"iat": nil}), "kid-1"), "c", ""},
		"no id token":          {"", "c", "GOOGLE_LOGIN_FAILED"},
		"nonce mismatch":       {f.sign(claims(jwt.MapClaims{"nonce": "other"}), "kid-1"), "c", "GOOGLE_LOGIN_FAILED"},
		"unverified email":     {f.sign(claims(jwt.MapClaims{"email_verified": false}), "kid-1"), "c", "GOOGLE_LOGIN_FAILED"},
		"missing subject":      {f.sign(claims(jwt.MapClaims{"sub": nil}), "kid-1"), "c", "GOOGLE_LOGIN_FAILED"},
		"non-Google mailbox":   {f.sign(claims(jwt.MapClaims{"email": "member@example.org"}), "kid-1"), "c", "GOOGLE_EMAIL_PASSWORD_REQUIRED"},
		"workspace account ok": {f.sign(claims(jwt.MapClaims{"email": "member@example.org", "hd": "example.org"}), "kid-1"), "c", "ok"},
	}
	for name, r := range refusals {
		f.idToken = r.token
		_, err := c.VerifyCode(context.Background(), r.code, "v", nonce)
		var refused *LoginError
		switch {
		case r.want == "ok":
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
		case r.want == "":
			if err == nil || errors.As(err, &refused) {
				t.Errorf("%s: got %v, want a plain failure", name, err)
			}
		default:
			if !errors.As(err, &refused) || refused.Code != r.want {
				t.Errorf("%s: got %v, want %s", name, err, r.want)
			}
		}
	}
}

func TestReadIdentityName(t *testing.T) {
	nonce := "n"
	base := map[string]any{"sub": "1", "email": "a@gmail.com", "email_verified": true, "nonce": nonce}
	for name, want := range map[any]string{nil: "a", "": "a", "   ": "a", strings.Repeat("é", 300): strings.Repeat("é", 255)} {
		c := map[string]any{}
		for k, v := range base {
			c[k] = v
		}
		if name != nil {
			c["name"] = name
		}
		identity, err := ReadIdentity(c, nonce)
		if err != nil || identity.Name != want {
			t.Errorf("name %v: got %q %v", name, identity.Name, err)
		}
	}
}

func TestVerifyCodeForSendsTheGivenRedirect(t *testing.T) {
	f := newFakeGoogle(t)
	f.idToken = f.sign(claims(nil), "kid-1")
	if _, err := f.client().VerifyCodeFor(context.Background(), "the-code", strings.Repeat("v", 43), strings.Repeat("n", 32), "https://perintis.test/masuk/google"); err != nil {
		t.Fatal(err)
	}
	if f.form["redirect_uri"] != "https://perintis.test/masuk/google" {
		t.Fatalf("redirect_uri = %q", f.form["redirect_uri"])
	}
}
