package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"time"
)

func RandomToken(bytes int) string {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func TokenHash(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func HashEqual(left, right []byte) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare(left, right) == 1
}

func SessionTTL(remember bool, idleMinutes, rememberDays int) time.Duration {
	if remember {
		return time.Duration(rememberDays) * 24 * time.Hour
	}
	return time.Duration(idleMinutes) * time.Minute
}
