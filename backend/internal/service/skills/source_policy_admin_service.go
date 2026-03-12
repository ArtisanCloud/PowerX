package skills

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	settingrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	ErrSkillSourcePolicyInvalid = errors.New("skill source policy invalid")
)

type SourcePolicyView struct {
	Allowlist       []string   `json:"allowlist"`
	EffectiveSource string     `json:"effective_source"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
}

type SourcePolicyAdminService struct {
	tenantSettings *settingrepo.TenantSettingRepository
}

func NewSourcePolicyAdminService(db *gorm.DB) *SourcePolicyAdminService {
	if db == nil {
		return nil
	}
	return &SourcePolicyAdminService{
		tenantSettings: settingrepo.NewTenantSettingRepository(db),
	}
}

func (s *SourcePolicyAdminService) GetTenantSourcePolicy(ctx context.Context, tenantUUID string) (*SourcePolicyView, error) {
	if s == nil || s.tenantSettings == nil {
		return &SourcePolicyView{
			Allowlist:       defaultAllowedSources(),
			EffectiveSource: "default",
		}, nil
	}
	setting, err := s.tenantSettings.GetByTenantAndKey(ctx, tenantUUID, TenantSettingKeySkillSourceAllowlist)
	if err != nil {
		return nil, err
	}
	if setting == nil {
		return &SourcePolicyView{
			Allowlist:       defaultAllowedSources(),
			EffectiveSource: "default",
		}, nil
	}
	allowlist := parseSourcesFromTenantSetting(setting)
	if len(allowlist) == 0 {
		allowlist = defaultAllowedSources()
	}
	updatedAt := setting.UpdatedAt
	return &SourcePolicyView{
		Allowlist:       allowlist,
		EffectiveSource: "tenant",
		UpdatedAt:       &updatedAt,
	}, nil
}

func (s *SourcePolicyAdminService) SetTenantSourcePolicy(ctx context.Context, tenantUUID string, allowlist []string) (*SourcePolicyView, error) {
	if s == nil || s.tenantSettings == nil {
		return nil, gorm.ErrInvalidDB
	}
	normalized := normalizeAllowedSources(allowlist)
	if len(normalized) == 0 {
		return nil, ErrSkillSourcePolicyInvalid
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	model := &dbsetting.TenantSetting{
		TenantUUID: tenantUUID,
		Key:        TenantSettingKeySkillSourceAllowlist,
		ValueJSON:  datatypes.JSON(raw),
		Group:      "ai",
		Editable:   true,
	}
	if err := s.tenantSettings.Upsert(ctx, model); err != nil {
		return nil, err
	}
	return s.GetTenantSourcePolicy(ctx, tenantUUID)
}
