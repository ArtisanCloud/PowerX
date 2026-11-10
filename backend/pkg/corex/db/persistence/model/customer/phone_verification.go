package customer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

const (
	PhoneVerifiedAtKey   = "phone_verified_at"
	PhoneVerifiedViaKey  = "phone_verified_via"
	PhoneVerifiedHashKey = "phone_verified_hash"
)

// PhoneVerificationHash 将认证证据绑定到具体号码，不在 metadata 中重复保存明文。
func PhoneVerificationHash(phone string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(phone)))
	return hex.EncodeToString(digest[:])
}

// HasVerifiedPhone 只信任与当前号码匹配的认证证据；旧的无号码标记不能证明当前号码。
func (a Account) HasVerifiedPhone() bool {
	if strings.TrimSpace(a.PrimaryPhone) == "" {
		return false
	}
	var evidence struct {
		At   string `json:"phone_verified_at"`
		Via  string `json:"phone_verified_via"`
		Hash string `json:"phone_verified_hash"`
	}
	if json.Unmarshal(a.Metadata, &evidence) != nil || strings.TrimSpace(evidence.Via) == "" || evidence.Hash != PhoneVerificationHash(a.PrimaryPhone) {
		return false
	}
	_, err := time.Parse(time.RFC3339, evidence.At)
	return err == nil
}
