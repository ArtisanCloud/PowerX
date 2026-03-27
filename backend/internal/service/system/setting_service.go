package system

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

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
func (s *SettingService) ListTenant(ctx context.Context, tenantUUID, prefix string, page, size int) (items []*dbsetting.TenantSetting, total int64, err error) {
	db := s.db.WithContext(ctx)
	if debug, ok := ctx.Value(utils.DebugKey).(bool); ok && debug {
		db = db.Debug()
	}

	q := db.Model(&dbsetting.TenantSetting{}).Where("tenant_uuid = ?", tenantUUID)
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

func (s *SettingService) GetTenant(ctx context.Context, tenantUUID, key string) (*dbsetting.TenantSetting, error) {
	return s.tenantRepo.GetByTenantAndKey(ctx, tenantUUID, key)
}

func (s *SettingService) UpsertTenant(ctx context.Context, tenantUUID, key string, value datatypes.JSON, group, desc *string, editable *bool) error {
	m := &dbsetting.TenantSetting{
		TenantUUID: tenantUUID,
		Key:        key,
		ValueJSON:  value,
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

func (s *SettingService) DeleteTenant(ctx context.Context, tenantUUID, key string, soft bool) error {
	return s.tenantRepo.Delete(ctx, tenantUUID, key, soft)
}

// =============== Effective (DB 层) ===============
func (s *SettingService) GetEffectiveFromDB(ctx context.Context, tenantUUID *string, key string) (json.RawMessage, string, error) {
	return s.tenantRepo.GetEffective(ctx, tenantUUID, key, s.sysRepo)
}

func (s *SettingService) GetSystemJSON(ctx context.Context, key string, out any) (bool, error) {
	item, err := s.GetSystem(ctx, key)
	if err != nil {
		return false, err
	}
	if item == nil || len(item.ValueJSON) == 0 {
		return false, nil
	}
	if out == nil {
		return false, errors.New("out cannot be nil")
	}
	if err := json.Unmarshal(item.ValueJSON, out); err != nil {
		return false, err
	}
	return true, nil
}

func (s *SettingService) UpsertSystemJSON(ctx context.Context, key string, value any, group, desc string, editable bool) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return errors.New("key cannot be empty")
	}
	bs, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.UpsertSystem(ctx, key, datatypes.JSON(bs), &group, &desc, &editable)
}
