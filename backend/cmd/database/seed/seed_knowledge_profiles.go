package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	tenantModel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	tenantRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type profileSeed struct {
	Key         string
	DisplayName string
	Config      map[string]any
}

func SeedKnowledgeProfiles(db *gorm.DB, tenantKey string) error {
	tenRepo := tenantRepo.NewTenantRepository(db)
	ten, err := tenRepo.EnsureByKey(seedCtx(), tenantKey, "SME Org", tenantModel.TenantPlanBasic, tenantModel.TenantTypeEnterprise)
	if err != nil {
		return fmt.Errorf("ensure tenant(%s): %w", tenantKey, err)
	}

	items := []profileSeed{
		{Key: "default", DisplayName: "Default", Config: map[string]any{"bundle": "p1_general"}},
		{Key: "p0_basic", DisplayName: "P0 基础（最小闭环）", Config: map[string]any{"bundle": "p0_basic"}},
		{Key: "p1_general", DisplayName: "P1 通用推荐（企业默认）", Config: map[string]any{"bundle": "p1_general"}},
		{Key: "p2_high_accuracy", DisplayName: "P2 高准确/合规（证据优先）", Config: map[string]any{"bundle": "p2_high_accuracy"}},
		{Key: "p3_kg_strong", DisplayName: "P3 KG 约束（关系驱动）", Config: map[string]any{"bundle": "p3_kg_strong"}},
	}

	now := time.Now().UTC()
	for _, it := range items {
		raw, _ := json.Marshal(it.Config)
		cfg := datatypes.JSON(raw)

		if err := ensurePublishedIngestionProfile(db, ten.UUID.String(), it.Key, it.DisplayName, cfg, &now); err != nil {
			return err
		}
		if err := ensurePublishedIndexProfile(db, ten.UUID.String(), it.Key, it.DisplayName, cfg, &now); err != nil {
			return err
		}
		if err := ensurePublishedRAGProfile(db, ten.UUID.String(), it.Key, it.DisplayName, cfg, &now); err != nil {
			return err
		}
	}

	logger.InfoF(context.Background(), "[seed] knowledge profiles ready for tenant=%s (uuid=%s)", tenantKey, ten.UUID.String())
	return nil
}

func ensurePublishedIngestionProfile(db *gorm.DB, tenantUUID, key, name string, cfg datatypes.JSON, now *time.Time) error {
	var existing models.IngestionProfileVersion
	err := db.WithContext(seedCtx()).
		Where("tenant_uuid = ? AND profile_key = ? AND status = ?", tenantUUID, key, models.ProfileStatusPublished).
		Order("version desc").
		Take(&existing).Error
	if err == nil && existing.UUID.String() != "" {
		return nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	row := &models.IngestionProfileVersion{
		TenantUUID:  tenantUUID,
		ProfileKey:  key,
		Version:     1,
		Status:      models.ProfileStatusPublished,
		DisplayName: name,
		Config:      cfg,
		PublishedAt: now,
		PublishedBy: "seed",
		CreatedBy:   "seed",
	}
	return db.WithContext(seedCtx()).Create(row).Error
}

func ensurePublishedIndexProfile(db *gorm.DB, tenantUUID, key, name string, cfg datatypes.JSON, now *time.Time) error {
	var existing models.IndexProfileVersion
	err := db.WithContext(seedCtx()).
		Where("tenant_uuid = ? AND profile_key = ? AND status = ?", tenantUUID, key, models.ProfileStatusPublished).
		Order("version desc").
		Take(&existing).Error
	if err == nil && existing.UUID.String() != "" {
		return nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	row := &models.IndexProfileVersion{
		TenantUUID:  tenantUUID,
		ProfileKey:  key,
		Version:     1,
		Status:      models.ProfileStatusPublished,
		DisplayName: name,
		Config:      cfg,
		PublishedAt: now,
		PublishedBy: "seed",
		CreatedBy:   "seed",
	}
	return db.WithContext(seedCtx()).Create(row).Error
}

func ensurePublishedRAGProfile(db *gorm.DB, tenantUUID, key, name string, cfg datatypes.JSON, now *time.Time) error {
	var existing models.RAGProfileVersion
	err := db.WithContext(seedCtx()).
		Where("tenant_uuid = ? AND profile_key = ? AND status = ?", tenantUUID, key, models.ProfileStatusPublished).
		Order("version desc").
		Take(&existing).Error
	if err == nil && existing.UUID.String() != "" {
		return nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	row := &models.RAGProfileVersion{
		TenantUUID:  tenantUUID,
		ProfileKey:  key,
		Version:     1,
		Status:      models.ProfileStatusPublished,
		DisplayName: name,
		Config:      cfg,
		PublishedAt: now,
		PublishedBy: "seed",
		CreatedBy:   "seed",
	}
	return db.WithContext(seedCtx()).Create(row).Error
}
