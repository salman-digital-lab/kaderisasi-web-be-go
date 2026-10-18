package auth

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestPublicAccessAdonisInteroperability(t *testing.T) {
	raw, err := exec.Command("node", "../../scripts/crypto-fixture.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("fixture: %v: %s", err, raw)
	}
	var f struct{ Key, PublicAccess, LegacyPublicAccess string }
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1700000100, 0)
	email := "fixture@example.test"
	signed, err := SignPublicAccess(f.Key, 42, &email, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if signed != f.PublicAccess {
		t.Fatalf("learner token differs from Adonis:\n go:    %s\n adonis: %s", signed, f.PublicAccess)
	}
	for name, token := range map[string]string{"adonis": f.PublicAccess, "legacy": f.LegacyPublicAccess} {
		claims, err := VerifyPublicAccess(f.Key, token, now)
		if err != nil || claims.UserID != 42 || claims.Email == nil || *claims.Email != email {
			t.Fatalf("%s learner token rejected: %v %+v", name, err, claims)
		}
		if claims.HasAudience != (name == "adonis") {
			t.Fatalf("%s audience flag %v", name, claims.HasAudience)
		}
	}
	if _, err = VerifyPublicAccess(f.Key, f.PublicAccess, now.Add(PublicAccessTTL)); err == nil {
		t.Fatal("expired learner token accepted")
	}
	if _, err = VerifyPublicAccess("different-test-signing-key", f.PublicAccess, now); err == nil {
		t.Fatal("learner token with a different key accepted")
	}
	data, _ := json.Marshal(struct {
		PublicAccess string `json:"publicAccess"`
	}{signed})
	command := exec.Command("node", "../../scripts/crypto-fixture.cjs", "verify")
	command.Stdin = bytes.NewReader(data)
	if output, err := command.CombinedOutput(); err != nil || strings.TrimSpace(string(output)) != "verified" {
		t.Fatalf("reverse learner token interoperability: %v %s", err, output)
	}
}

func TestVerifyPublicAccessRejections(t *testing.T) {
	key := "synthetic-test-key-never-use-outside-tests"
	now := time.Unix(1700000100, 0)
	sign := func(method jwt.SigningMethod, claims jwt.MapClaims) string {
		token, err := jwt.NewWithClaims(method, claims).SignedString([]byte(key))
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	cases := map[string]string{
		"administrator audience": sign(jwt.SigningMethodHS256, jwt.MapClaims{"userId": 42, "aud": "kaderisasi-admin", "exp": now.Unix() + 900}),
		"foreign audience":       sign(jwt.SigningMethodHS256, jwt.MapClaims{"userId": 42, "aud": "other", "exp": now.Unix() + 60}),
		"two audiences":          sign(jwt.SigningMethodHS256, jwt.MapClaims{"userId": 42, "aud": []string{PublicAudience, "other"}, "exp": now.Unix() + 60}),
		"missing user":           sign(jwt.SigningMethodHS256, jwt.MapClaims{"aud": PublicAudience, "exp": now.Unix() + 60}),
		"string user":            sign(jwt.SigningMethodHS256, jwt.MapClaims{"userId": "42", "aud": PublicAudience, "exp": now.Unix() + 60}),
		"fractional user":        sign(jwt.SigningMethodHS256, jwt.MapClaims{"userId": 4.5, "aud": PublicAudience, "exp": now.Unix() + 60}),
		"zero user":              sign(jwt.SigningMethodHS256, jwt.MapClaims{"userId": 0, "aud": PublicAudience, "exp": now.Unix() + 60}),
		"HS512":                  sign(jwt.SigningMethodHS512, jwt.MapClaims{"userId": 42, "aud": PublicAudience, "exp": now.Unix() + 60}),
		"expired":                sign(jwt.SigningMethodHS256, jwt.MapClaims{"userId": 42, "aud": PublicAudience, "exp": now.Unix() - 1}),
		"malformed":              "not-a-token",
	}
	for name, token := range cases {
		if _, err := VerifyPublicAccess(key, token, now); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	// Like the Adonis guard, a token without exp stays valid.
	if claims, err := VerifyPublicAccess(key, sign(jwt.SigningMethodHS256, jwt.MapClaims{"userId": 7}), now); err != nil || claims.UserID != 7 {
		t.Fatalf("token without exp rejected: %v", err)
	}
}
