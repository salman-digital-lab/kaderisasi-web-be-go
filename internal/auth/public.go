package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// PublicAudience and PublicAccessTTL match the web-be (Adonis) JWT guard:
// `@maximemrf/adonisjs-jwt` signs {userId, email, aud} with APP_KEY and a
// one-day expiry through `jsonwebtoken`.
const PublicAudience = "kaderisasi-public"
const PublicAccessTTL = 24 * time.Hour

// publicPayload keeps jsonwebtoken's claim order (content, then iat, exp) and
// its single-string audience so Go-issued learner tokens are byte-identical to
// Adonis-issued ones for the same inputs.
type publicPayload struct {
	UserID   int32   `json:"userId"`
	Email    *string `json:"email"`
	Audience string  `json:"aud"`
	IssuedAt int64   `json:"iat"`
	Expires  int64   `json:"exp"`
}

func (p publicPayload) GetExpirationTime() (*jwt.NumericDate, error) {
	return jwt.NewNumericDate(time.Unix(p.Expires, 0)), nil
}
func (p publicPayload) GetIssuedAt() (*jwt.NumericDate, error) {
	return jwt.NewNumericDate(time.Unix(p.IssuedAt, 0)), nil
}
func (publicPayload) GetNotBefore() (*jwt.NumericDate, error) { return nil, nil }
func (publicPayload) GetIssuer() (string, error)              { return "", nil }
func (publicPayload) GetSubject() (string, error)             { return "", nil }
func (p publicPayload) GetAudience() (jwt.ClaimStrings, error) {
	return jwt.ClaimStrings{p.Audience}, nil
}

// PublicClaims is a verified learner token. HasAudience is false for legacy
// tokens; course routes require the learner audience and an expiry explicitly.
type PublicClaims struct {
	UserID      int32
	Email       *string
	HasAudience bool
	HasExpiry   bool
}

func SignPublicAccess(key string, userID int32, email *string, now time.Time) (string, error) {
	issued := now.Unix()
	return jwt.NewWithClaims(jwt.SigningMethodHS256, publicPayload{UserID: userID, Email: email, Audience: PublicAudience, IssuedAt: issued, Expires: issued + int64(PublicAccessTTL/time.Second)}).SignedString([]byte(key))
}

type publicClaims struct {
	UserID *float64 `json:"userId"`
	Email  *string  `json:"email"`
	jwt.RegisteredClaims
}

// VerifyPublicAccess accepts what the Adonis guard accepts (`jwt.verify` with
// APP_KEY: signature, exp when present, a numeric userId), with two deliberate
// tightenings: only HS256 is accepted, and a token whose audience is not the
// learner audience (for example an administrator token) is rejected instead of
// being read as a learner id. Tokens issued before the audience claim existed
// carry no `aud` and stay valid.
func VerifyPublicAccess(key, token string, now time.Time) (PublicClaims, error) {
	var claims publicClaims
	_, err := jwt.ParseWithClaims(token, &claims, func(*jwt.Token) (interface{}, error) { return []byte(key), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil || claims.UserID == nil {
		return PublicClaims{}, errors.New("invalid learner token")
	}
	id := *claims.UserID
	if id != float64(int32(id)) || id <= 0 {
		return PublicClaims{}, errors.New("invalid learner id")
	}
	if len(claims.Audience) > 0 && !(len(claims.Audience) == 1 && claims.Audience[0] == PublicAudience) {
		return PublicClaims{}, errors.New("invalid learner audience")
	}
	return PublicClaims{UserID: int32(id), Email: claims.Email, HasAudience: len(claims.Audience) > 0, HasExpiry: claims.ExpiresAt != nil}, nil
}
