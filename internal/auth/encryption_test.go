package auth

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPasswordAndEncryptionAdonisInteroperability(t *testing.T) {
	raw, err := exec.Command("node", "../../scripts/crypto-fixture.cjs").CombinedOutput()
	if err != nil {
		t.Fatalf("fixture: %v: %s", err, raw)
	}
	var f struct{ Key, PublicAccess, Password, Hash, Encrypted, Expired, Unexpiring, Purposed, Object string }
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(f.Hash, f.Password) || VerifyPassword(f.Hash, "wrong") {
		t.Fatal("Adonis scrypt hash not verified")
	}
	time.Sleep(5 * time.Millisecond) // let the 1ms token expire
	now := time.Now()
	for name, token := range map[string]string{"expiring": f.Encrypted, "unexpiring": f.Unexpiring} {
		message, err := Decrypt(f.Key, token, now)
		if err != nil || string(message) != `"fixture@example.test"` {
			t.Fatalf("%s Adonis token: %s %v", name, message, err)
		}
	}
	if message, err := Decrypt(f.Key, f.Object, now); err != nil || string(message) != `{"email":"fixture@example.test"}` {
		t.Fatalf("object message: %s %v", message, err)
	}
	for name, token := range map[string]string{"expired": f.Expired, "purpose": f.Purposed, "wrong key": "", "tampered": f.Encrypted[:len(f.Encrypted)-2] + "AA",
		"two parts": strings.Join(strings.Split(f.Encrypted, ".")[:2], "."), "not base64url": "a+b.c.d"} {
		key := f.Key
		if name == "wrong key" {
			key, token = "different-test-signing-key", f.Encrypted
		}
		if _, err := Decrypt(key, token, now); err == nil {
			t.Errorf("%s token accepted", name)
		}
	}
	if _, err := Decrypt(f.Key, f.Encrypted, now.Add(31*time.Minute)); err == nil {
		t.Error("token accepted after its 30 minutes")
	}

	hash, err := HashPassword(f.Password)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := Encrypt(f.Key, "fixture@example.test", 30*time.Minute, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	expired, _ := Encrypt(f.Key, "fixture@example.test", time.Minute, time.Now().Add(-2*time.Minute))
	if message, err := Decrypt(f.Key, encrypted, time.Now()); err != nil || string(message) != `"fixture@example.test"` {
		t.Fatalf("Go round trip: %s %v", message, err)
	}
	signed, _ := SignPublicAccess(f.Key, 42, nil, time.Unix(1700000000, 0))
	data, _ := json.Marshal(map[string]string{"publicAccess": signed, "hash": hash, "encrypted": encrypted, "expired": expired})
	command := exec.Command("node", "../../scripts/crypto-fixture.cjs", "verify")
	command.Stdin = bytes.NewReader(data)
	if output, err := command.CombinedOutput(); err != nil || strings.TrimSpace(string(output)) != "verified" {
		t.Fatalf("reverse interoperability: %v %s", err, output)
	}
}
