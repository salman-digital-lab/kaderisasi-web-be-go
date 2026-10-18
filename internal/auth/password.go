package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"strconv"
	"strings"

	"golang.org/x/crypto/scrypt"
)

// HashPassword and VerifyPassword match the Adonis scrypt driver configured in
// kaderisasi-web-be/config/hash.ts (cost 16384, block size 8, parallelization 1)
// and its PHC string format. Copied from kaderisasi-admin-be-go/internal/auth.
func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash, err := scrypt.Key([]byte(password), salt, 16384, 8, 1, 64)
	if err != nil {
		return "", err
	}
	return "$scrypt$n=16384,r=8,p=1$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash), nil
}
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[1] != "scrypt" {
		return false
	}
	params := map[string]int{}
	for _, part := range strings.Split(parts[2], ",") {
		pair := strings.SplitN(part, "=", 2)
		if len(pair) != 2 {
			return false
		}
		n, err := strconv.Atoi(pair[1])
		if err != nil {
			return false
		}
		params[pair[0]] = n
	}
	n, r, p := params["n"], params["r"], params["p"]
	if n < 2 || n > 262144 || r < 1 || r > 32 || p < 1 || p > 16 || int64(n)*int64(r)*128 >= 33554432 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(salt) < 8 || len(salt) > 1024 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(expected) < 64 || len(expected) > 128 {
		return false
	}
	got, err := scrypt.Key([]byte(password), salt, n, r, p, len(expected))
	return err == nil && subtle.ConstantTimeCompare(got, expected) == 1
}
