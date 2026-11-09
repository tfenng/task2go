package params

import (
	"context"
	"testing"

	"github.com/tfenng/tasks/task2go/internal/security"
)

func testBox() *security.SecretBox {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	return security.NewSecretBox(map[int][]byte{1: key}, 1)
}

func TestDefaultsValidateRedact(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":  map[string]any{"type": "string", "default": "world"},
			"token": map[string]any{"type": "string", "x-secret": true},
			"count": map[string]any{"type": "integer", "minimum": float64(1)},
		},
		"required":             []any{"count"},
		"additionalProperties": true,
	}
	value := map[string]any{"count": float64(3), "token": "abc"}
	withDefaults := ApplyDefaults(schema, value)
	if withDefaults["name"] != "world" {
		t.Fatalf("default not applied: %#v", withDefaults)
	}
	if err := ValidateParameters(context.Background(), schema, withDefaults); err != nil {
		t.Fatal(err)
	}
	redacted := Redact(schema, withDefaults)
	if redacted["token"] != "***" {
		t.Fatalf("secret not redacted: %#v", redacted)
	}
	if err := ValidateParameters(context.Background(), schema, map[string]any{}); err == nil {
		t.Fatal("expected required field error")
	}
}

func TestEncryptDecryptParameters(t *testing.T) {
	box := testBox()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"token": map[string]any{"type": "string", "writeOnly": true},
		},
	}
	original := map[string]any{"token": "secret", "visible": "ok"}
	encrypted, err := EncryptParameters(schema, original, "run-1", box)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := encrypted["token"].(map[string]any); !ok {
		t.Fatalf("token was not encrypted: %#v", encrypted["token"])
	}
	decrypted, err := DecryptParameters(schema, encrypted, "run-1", box)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted["token"] != "secret" {
		t.Fatalf("got %#v", decrypted)
	}
}

func TestCanonicalJSON(t *testing.T) {
	left := map[string]any{"b": 2, "a": 1}
	right := map[string]any{"a": 1, "b": 2}
	l, _ := CanonicalJSON(left)
	r, _ := CanonicalJSON(right)
	if string(l) != string(r) {
		t.Fatalf("%s != %s", l, r)
	}
}
