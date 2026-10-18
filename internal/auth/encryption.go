package auth

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"
)

// Encrypt and Decrypt match `encryption.encrypt(value, expiresIn)` and
// `encryption.decrypt(value)` from @adonisjs/encryption 6 with APP_KEY and no
// purpose: AES-256-CBC keyed by sha256(APP_KEY) over the JSON message
// {"message", "expiryDate"}, formatted as
// base64url(ciphertext).base64url(iv).base64url(hmac-sha256(key, first two parts)).
// web-be uses them for password reset links (30 minutes).

type encryptedMessage struct {
	Message    json.RawMessage `json:"message"`
	Purpose    *string         `json:"purpose,omitempty"`
	ExpiryDate *string         `json:"expiryDate,omitempty"`
}

// jsISO is Date.prototype.toISOString.
func jsISO(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

const randomAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

func Encrypt(key, value string, expiresIn time.Duration, now time.Time) (string, error) {
	message, _ := json.Marshal(value)
	payload := encryptedMessage{Message: message}
	if expiresIn > 0 {
		expiry := jsISO(now.Add(expiresIn))
		payload.ExpiryDate = &expiry
	}
	plain, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	// Adonis uses string.random(16): sixteen URL-safe characters as the IV bytes.
	iv := make([]byte, aes.BlockSize)
	for i := range iv {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(randomAlphabet))))
		if err != nil {
			return "", err
		}
		iv[i] = randomAlphabet[n.Int64()]
	}
	secret := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(secret[:])
	if err != nil {
		return "", err
	}
	padding := aes.BlockSize - len(plain)%aes.BlockSize
	plain = append(plain, bytes.Repeat([]byte{byte(padding)}, padding)...)
	sealed := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(sealed, plain)
	head := base64.RawURLEncoding.EncodeToString(sealed) + "." + base64.RawURLEncoding.EncodeToString(iv)
	return head + "." + encryptionMAC(secret[:], head), nil
}

func encryptionMAC(secret []byte, value string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

var errInvalidEncryption = errors.New("invalid encrypted value")

// Decrypt returns the decrypted message (any JSON value) or an error where
// Adonis returns null.
func Decrypt(key, value string, now time.Time) (json.RawMessage, error) {
	parts := strings.Split(value, ".")
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, errInvalidEncryption
	}
	sealed, err := urlDecode(parts[0])
	if err != nil {
		return nil, err
	}
	iv, err := urlDecode(parts[1])
	if err != nil {
		return nil, err
	}
	secret := sha256.Sum256([]byte(key))
	if !hmac.Equal([]byte(encryptionMAC(secret[:], parts[0]+"."+parts[1])), []byte(parts[2])) {
		return nil, errInvalidEncryption
	}
	block, err := aes.NewCipher(secret[:])
	if err != nil || len(iv) != aes.BlockSize || len(sealed) == 0 || len(sealed)%aes.BlockSize != 0 {
		return nil, errInvalidEncryption
	}
	plain := make([]byte, len(sealed))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, sealed)
	padding := int(plain[len(plain)-1])
	if padding == 0 || padding > aes.BlockSize || !bytes.Equal(plain[len(plain)-padding:], bytes.Repeat([]byte{byte(padding)}, padding)) {
		return nil, errInvalidEncryption
	}
	var message encryptedMessage
	if err := json.Unmarshal(plain[:len(plain)-padding], &message); err != nil {
		return nil, errInvalidEncryption
	}
	// MessageBuilder.verify: a falsy message, any purpose, or an unparsable or
	// past expiry date is rejected.
	switch strings.TrimSpace(string(message.Message)) {
	case "", "null", "false", "0", `""`:
		return nil, errInvalidEncryption
	}
	if message.Purpose != nil {
		return nil, errInvalidEncryption
	}
	if message.ExpiryDate != nil {
		expiry, err := time.Parse(time.RFC3339Nano, *message.ExpiryDate)
		if err != nil || expiry.Before(now) {
			return nil, errInvalidEncryption
		}
	}
	return message.Message, nil
}

// urlDecode is @poppinss/utils base64.urlDecode (non-strict): it accepts only
// values that re-encode to themselves.
func urlDecode(value string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, errInvalidEncryption
	}
	return decoded, nil
}
