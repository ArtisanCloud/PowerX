package skills

import (
	"errors"
	"os"
	"strings"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
)

// IntegrityPolicy controls checksum/signature requirements.
type IntegrityPolicy struct {
	RequireChecksum  bool
	RequireSignature bool
}

func NewIntegrityPolicyFromEnv() *IntegrityPolicy {
	return &IntegrityPolicy{
		RequireChecksum:  readBoolEnv("POWERX_SKILL_REQUIRE_CHECKSUM", true),
		RequireSignature: readBoolEnv("POWERX_SKILL_REQUIRE_SIGNATURE", false),
	}
}

func (p *IntegrityPolicy) ValidateImport(req ImportRequest) error {
	policy := p.effective()
	if policy.RequireChecksum && strings.TrimSpace(req.Checksum) == "" {
		return errors.New("checksum is required")
	}
	if checksum := strings.TrimSpace(req.Checksum); checksum != "" && !isChecksumAccepted(checksum) {
		return errors.New("checksum mismatch: only sha256 digest is accepted")
	}
	if policy.RequireSignature && strings.TrimSpace(req.Signature) == "" {
		return errors.New("signature is required by integrity policy")
	}
	return nil
}

func (p *IntegrityPolicy) ValidatePublish(record *skillmodel.SkillRegistryRecord) error {
	if record == nil {
		return errors.New("skill record is required")
	}
	policy := p.effective()
	if policy.RequireChecksum && strings.TrimSpace(record.Checksum) == "" {
		return errors.New("checksum is required before publish")
	}
	if checksum := strings.TrimSpace(record.Checksum); checksum != "" && !isChecksumAccepted(checksum) {
		return errors.New("checksum mismatch: only sha256 digest is accepted")
	}
	if policy.RequireSignature && strings.TrimSpace(record.Signature) == "" {
		return errors.New("signature is required before publish")
	}
	return nil
}

func (p *IntegrityPolicy) effective() *IntegrityPolicy {
	if p == nil {
		return &IntegrityPolicy{RequireChecksum: true, RequireSignature: false}
	}
	return p
}

func isChecksumAccepted(checksum string) bool {
	lower := strings.ToLower(strings.TrimSpace(checksum))
	return strings.HasPrefix(lower, "sha256:") || strings.HasPrefix(lower, "sha256-")
}

func readBoolEnv(key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
