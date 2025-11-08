package utils

import (
	"crypto/sha256"
	"encoding/hex"
)

// utils/jwt.go

func SecretFP(secret []byte) string {
	h := sha256.Sum256(secret)
	return hex.EncodeToString(h[:4]) + "…" + hex.EncodeToString(h[len(h)-4:])
}
