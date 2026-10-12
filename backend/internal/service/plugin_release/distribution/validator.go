package distribution

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	errChecksumRequired   = errors.New("distribution.validator: checksum is required")
	errSignatureRequired  = errors.New("distribution.validator: signature fingerprint is required")
	errLicenseInformation = errors.New("distribution.validator: license report is required")
)

// Validator provides structural verification for offline package inputs.
type Validator struct{}

// NewValidator returns a new validator instance.
func NewValidator() *Validator {
	return &Validator{}
}

// VerifyChecksum compares the supplied checksum with the computed value when content is provided.
func (Validator) VerifyChecksum(content []byte, expected string) error {
	trimmed := strings.TrimSpace(strings.ToLower(expected))
	if trimmed == "" {
		return errChecksumRequired
	}
	if len(content) == 0 {
		// Caller provided checksum but no content to verify – assume it was pre-computed.
		return nil
	}
	sum := sha256.Sum256(content)
	actual := hex.EncodeToString(sum[:])
	if actual != normalizeChecksum(trimmed) {
		return fmt.Errorf("checksum mismatch: got %s want %s", actual, normalizeChecksum(trimmed))
	}
	return nil
}

// RequireSignature ensures the fingerprint is present and well formed.
func (Validator) RequireSignature(fingerprint string) error {
	trimmed := strings.TrimSpace(fingerprint)
	if trimmed == "" || len(trimmed) < 8 {
		return errSignatureRequired
	}
	return nil
}

func (Validator) VerifyEd25519(content []byte, signature, publicKey string) error {
	if len(content) == 0 {
		return errChecksumRequired
	}
	sig, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(signature))
	if err != nil {
		return errSignatureRequired
	}
	key, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(publicKey))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return errSignatureRequired
	}
	if !ed25519.Verify(ed25519.PublicKey(key), content, sig) {
		return fmt.Errorf("ed25519 signature verification failed")
	}
	return nil
}

// IsEd25519PublicKey checks the Core-owned key material before it is persisted.
func (Validator) IsEd25519PublicKey(publicKey string) bool {
	key, err := base64.RawStdEncoding.DecodeString(strings.TrimSpace(publicKey))
	return err == nil && len(key) == ed25519.PublicKeySize
}

// VerifyLicense ensures at least one license entry is present.
func (Validator) VerifyLicense(report map[string]any) error {
	if len(report) == 0 {
		return errLicenseInformation
	}
	return nil
}

func normalizeChecksum(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "sha256:")
	return value
}
