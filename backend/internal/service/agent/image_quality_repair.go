package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	am "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ImageQualityRepairPlan 只修复已确认的 dev/image/openai/gpt-image-2 旧值。
// 完整默认参数只参与摘要，不写入报告；旧任务及候选快照不在此操作范围内。
type ImageQualityRepairPlan struct {
	Schema     string `json:"schema"`
	TenantUUID string `json:"tenant_uuid"`
	ProfileID  uint64 `json:"profile_id"`
	Env        string `json:"env"`
	Modality   string `json:"modality"`
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Before     string `json:"before_quality"`
	After      string `json:"after_quality"`
	BeforeHash string `json:"before_defaults_hash"`
	AfterHash  string `json:"after_defaults_hash"`
	Eligible   bool   `json:"eligible"`
}

func imageDefaultsHash(values datatypes.JSONMap) string {
	raw, _ := json.Marshal(values)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func imageQualityRepairTarget(plan ImageQualityRepairPlan) bool {
	_, err := uuid.Parse(plan.TenantUUID)
	return err == nil && plan.Schema == "powerx.ai.image-quality-repair/v1" && plan.Env == "dev" && plan.Modality == "image" && plan.Provider == "openai" && plan.Model == "gpt-image-2"
}

// PreviewImageQualityRepair 不写配置、不查询供应商。无匹配画像或存在多条时明确拒绝。
func (s *AgentSettingService) PreviewImageQualityRepair(ctx context.Context, tenant string) (*ImageQualityRepairPlan, error) {
	plan := &ImageQualityRepairPlan{Schema: "powerx.ai.image-quality-repair/v1", TenantUUID: tenant, Env: "dev", Modality: "image", Provider: "openai", Model: "gpt-image-2", After: "auto"}
	if !imageQualityRepairTarget(*plan) {
		return nil, &dto.AppError{HTTPCode: 400, Code: "AI_IMAGE_REPAIR_SCOPE_INVALID", Message: "AI_IMAGE_REPAIR_SCOPE_INVALID"}
	}
	var profiles []am.AIModelProfile
	if err := s.db.WithContext(ctx).Where("tenant_uuid=? AND env=? AND modality=? AND provider=? AND model=?", tenant, plan.Env, plan.Modality, plan.Provider, plan.Model).Find(&profiles).Error; err != nil {
		return nil, &dto.AppError{HTTPCode: 503, Code: "AI_IMAGE_REPAIR_READ_FAILED", Message: "AI_IMAGE_REPAIR_READ_FAILED"}
	}
	if len(profiles) != 1 {
		return nil, &dto.AppError{HTTPCode: 409, Code: "AI_IMAGE_REPAIR_TARGET_AMBIGUOUS", Message: "AI_IMAGE_REPAIR_TARGET_AMBIGUOUS"}
	}
	profile := profiles[0]
	plan.ProfileID, plan.BeforeHash = profile.ID, imageDefaultsHash(profile.Defaults)
	plan.Before, _ = profile.Defaults["quality"].(string)
	plan.Eligible = plan.Before == "standard"
	if plan.Eligible {
		proposed := datatypes.JSONMap{}
		for key, value := range profile.Defaults {
			proposed[key] = value
		}
		proposed["quality"] = "auto"
		manifest := findModelManifest("image", "openai", "gpt-image-2")
		if manifest == nil {
			return nil, errors.New("AI_IMAGE_REPAIR_CONTRACT_MISSING")
		}
		if _, reason := catalog.ValidateMediaParameters(manifest.Operations[catalog.ImageGeneration], catalog.MediaDefaults(manifest, catalog.ImageGeneration, proposed)); reason != "" {
			return nil, errors.New("AI_IMAGE_REPAIR_OTHER_DEFAULTS_INVALID")
		}
		plan.AfterHash = imageDefaultsHash(proposed)
	}
	return plan, nil
}

// ApplyImageQualityRepair 锁定目标并校验预览摘要；重复执行不覆盖任何后续人工调整。
func (s *AgentSettingService) ApplyImageQualityRepair(ctx context.Context, plan ImageQualityRepairPlan) (bool, error) {
	if !imageQualityRepairTarget(plan) || !plan.Eligible || plan.ProfileID == 0 || plan.Before != "standard" || plan.After != "auto" || len(plan.BeforeHash) != 64 || len(plan.AfterHash) != 64 {
		return false, errors.New("AI_IMAGE_REPAIR_PLAN_INVALID")
	}
	applied := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var profile am.AIModelProfile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND tenant_uuid=? AND env=? AND modality=? AND provider=? AND model=?", plan.ProfileID, plan.TenantUUID, plan.Env, plan.Modality, plan.Provider, plan.Model).First(&profile).Error; err != nil {
			return errors.New("AI_IMAGE_REPAIR_TARGET_CHANGED")
		}
		current := imageDefaultsHash(profile.Defaults)
		if current == plan.AfterHash && profile.Defaults["quality"] == "auto" {
			return nil
		}
		if current != plan.BeforeHash || profile.Defaults["quality"] != "standard" {
			return errors.New("AI_IMAGE_REPAIR_PREVIEW_STALE")
		}
		proposed := datatypes.JSONMap{}
		for key, value := range profile.Defaults {
			proposed[key] = value
		}
		proposed["quality"] = "auto"
		if imageDefaultsHash(proposed) != plan.AfterHash {
			return errors.New("AI_IMAGE_REPAIR_PLAN_INVALID")
		}
		check, err := NewAgentSettingService(tx).PreviewImageQualityRepair(ctx, plan.TenantUUID)
		if err != nil || check.BeforeHash != plan.BeforeHash || check.AfterHash != plan.AfterHash {
			return errors.New("AI_IMAGE_REPAIR_PREVIEW_STALE")
		}
		if err := tx.Model(&profile).Update("defaults", proposed).Error; err != nil {
			return errors.New("AI_IMAGE_REPAIR_WRITE_FAILED")
		}
		applied = true
		return nil
	})
	return err == nil && applied, err
}
