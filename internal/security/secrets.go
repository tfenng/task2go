package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
)

type SecretBox struct {
	KeyRing       map[int][]byte
	ActiveVersion int
}

func NewSecretBox(keyRing map[int][]byte, active int) *SecretBox {
	return &SecretBox{KeyRing: keyRing, ActiveVersion: active}
}

type encryptedPayload struct {
	Enc        string `json:"__enc__"`
	KeyVersion int    `json:"key_version"`
}

func (b *SecretBox) Encrypt(plaintext, aad string) (map[string]any, error) {
	key, ok := b.KeyRing[b.ActiveVersion]
	if !ok {
		return nil, fmt.Errorf("active secret version %d missing", b.ActiveVersion)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), []byte(aad))
	raw := append(nonce, ciphertext...)
	return map[string]any{
		"__enc__":     base64.StdEncoding.EncodeToString(raw),
		"key_version": b.ActiveVersion,
	}, nil
}

func (b *SecretBox) Decrypt(payload map[string]any, aad string) (string, error) {
	encoded, _ := payload["__enc__"].(string)
	version, err := intValue(payload["key_version"])
	if err != nil || encoded == "" {
		return "", ErrInvalidSecret
	}
	key, ok := b.KeyRing[version]
	if !ok {
		return "", ErrInvalidSecret
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrInvalidSecret
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", ErrInvalidSecret
	}
	plaintext, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte(aad))
	if err != nil {
		return "", ErrInvalidSecret
	}
	return string(plaintext), nil
}

func intValue(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	case json.Number:
		i, err := strconv.Atoi(n.String())
		return i, err
	default:
		return 0, fmt.Errorf("not a number")
	}
}
