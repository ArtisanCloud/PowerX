package system

import (
	"context"
	"encoding/json"
	"gorm.io/datatypes"

	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gorm.io/gorm"
)

type SettingService struct {
	db         *gorm.DB
	sysRepo    *repo.SystemSettingRepository
	tenantRepo *repo.TenantSettingRepository
}

func NewSettingService(db *gorm.DB) *SettingService {
	return &SettingService{
		db:         db,
		sysRepo:    repo.NewSystemSettingRepository(db),
		tenantRepo: repo.NewTenantSettingRepository(db),
	}
}

// =============== System ===============
func (s *SettingService) ListSystem(ctx context.Context, group, prefix string, page, size int) (items []*dbsetting.SystemSetting, total int64, err error) {
	db := s.db.WithContext(ctx)
	if debug, ok := ctx.Value(utils.DebugKey).(bool); ok && debug {
		db = db.Debug()
	}

	q := db.Model(&dbsetting.SystemSetting{})
	if group != "" {
		q = q.Where("group = ?", group)
	}
	if prefix != "" {
		q = q.Where("key LIKE ?", prefix+"%")
	}

	if err = q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err = q.Order("id DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return
}

func (s *SettingService) GetSystem(ctx context.Context, key string) (*dbsetting.SystemSetting, error) {
	return s.sysRepo.GetByKey(ctx, key)
}

func (s *SettingService) UpsertSystem(ctx context.Context, key string, value datatypes.JSON, group, desc *string, editable *bool) error {
	m := &dbsetting.SystemSetting{
		Key:       key,
		ValueJSON: value,
	}
	if group != nil {
		m.Group = *group
	}
	if desc != nil {
		m.Description = desc
	}
	if editable != nil {
		m.Editable = *editable
	}
	return s.sysRepo.Upsert(ctx, m)
}

func (s *SettingService) DeleteSystem(ctx context.Context, key string, soft bool) error {
	return s.sysRepo.DeleteByKey(ctx, key, soft)
}

// =============== Tenant ===============
func (s *SettingService) ListTenant(ctx context.Context, tenantID uint64, prefix string, page, size int) (items []*dbsetting.TenantSetting, total int64, err error) {
	db := s.db.WithContext(ctx)
	if debug, ok := ctx.Value(utils.DebugKey).(bool); ok && debug {
		db = db.Debug()
	}

	q := db.Model(&dbsetting.TenantSetting{}).Where("tenant_id = ?", tenantID)
	if prefix != "" {
		q = q.Where("key LIKE ?", prefix+"%")
	}

	if err = q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err = q.Order("id DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return
}

func (s *SettingService) GetTenant(ctx context.Context, tenantID uint64, key string) (*dbsetting.TenantSetting, error) {
	return s.tenantRepo.GetByTenantAndKey(ctx, tenantID, key)
}

func (s *SettingService) UpsertTenant(ctx context.Context, tenantID uint64, key string, value datatypes.JSON, group, desc *string, editable *bool) error {
	m := &dbsetting.TenantSetting{
		TenantID:  tenantID,
		Key:       key,
		ValueJSON: value,
	}
	if group != nil {
		m.Group = *group
	}
	if desc != nil {
		m.Description = desc
	}
	if editable != nil {
		m.Editable = *editable
	}
	return s.tenantRepo.Upsert(ctx, m)
}

func (s *SettingService) DeleteTenant(ctx context.Context, tenantID uint64, key string, soft bool) error {
	return s.tenantRepo.Delete(ctx, tenantID, key, soft)
}

// =============== Effective (DB 层) ===============
func (s *SettingService) GetEffectiveFromDB(ctx context.Context, tenantID *uint64, key string) (json.RawMessage, string, error) {
	return s.tenantRepo.GetEffective(ctx, tenantID, key, s.sysRepo)
}
