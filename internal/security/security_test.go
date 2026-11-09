package security

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("adminpassword123")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("adminpassword123", hash) {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword("wrong-password", hash) {
		t.Fatal("wrong password verified")
	}
}

func TestSecretBoxRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	box := NewSecretBox(map[int][]byte{1: key}, 1)
	payload, err := box.Encrypt("secret-value", "run:abc:param:token")
	if err != nil {
		t.Fatal(err)
	}
	got, err := box.Decrypt(payload, "run:abc:param:token")
	if err != nil {
		t.Fatal(err)
	}
	if got != "secret-value" {
		t.Fatalf("got %q", got)
	}
	if _, err := box.Decrypt(payload, "wrong-aad"); err == nil {
		t.Fatal("expected AAD mismatch")
	}
}

func TestPythonCompatibility(t *testing.T) {
	const pythonHash = "$argon2id$v=19$m=65536,t=3,p=4$JacbIs2xwYAkz/WiWFNqdA$GZI4j3xKS8h5M+LXhiKQrBL37D7lq3g5g6dMlUAqZEw"
	if !VerifyPassword("adminpassword123", pythonHash) {
		t.Fatal("python argon2id hash did not verify")
	}
	key, _ := base64.StdEncoding.DecodeString("AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	box := NewSecretBox(map[int][]byte{1: key}, 1)
	var payload map[string]any
	if err := json.Unmarshal([]byte(`{"__enc__":"AAECAwQFBgcICQoLNGe1aaCR723sLeLu8y4sTRlK1pP05UOfJhCoVA==","key_version":1}`), &payload); err != nil {
		t.Fatal(err)
	}
	got, err := box.Decrypt(payload, "run:abc:param:token")
	if err != nil {
		t.Fatal(err)
	}
	if got != "secret-value" {
		t.Fatalf("got %q", got)
	}
}

func TestSessionHash(t *testing.T) {
	token := RandomToken(48)
	if len(token) != 64 {
		t.Fatalf("unexpected token length: %d", len(token))
	}
	if !HashEqual(TokenHash(token), TokenHash(token)) {
		t.Fatal("hash mismatch")
	}
	if HashEqual(TokenHash(token), TokenHash(token+"x")) {
		t.Fatal("different tokens matched")
	}
}
