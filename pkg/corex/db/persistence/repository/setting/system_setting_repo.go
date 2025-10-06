// pkg/corex/db/persistence/repository/setting/system_setting_repo.go
package setting

import (
	"context"

	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SystemSettingRepository struct{ db *gorm.DB }

func NewSystemSettingRepository(db *gorm.DB) *SystemSettingRepository {
	return &SystemSettingRepository{db: db}
}

func (r *SystemSettingRepository) with(ctx context.Context) *gorm.DB {
	db := r.db.WithContext(ctx)
	if debug, ok := ctx.Value(utils.DebugKey).(bool); ok && debug {
		db = db.Debug()
	}
	return db
}

func (r *SystemSettingRepository) GetByKey(ctx context.Context, key string) (*dbsetting.SystemSetting, error) {
	var s dbsetting.SystemSetting
	if err := r.with(ctx).Where("key = ?", key).First(&s).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

func (r *SystemSettingRepository) ListByGroup(ctx context.Context, group string) ([]*dbsetting.SystemSetting, error) {
	var list []*dbsetting.SystemSetting
	if err := r.with(ctx).Where("group = ?", group).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *SystemSettingRepository) Upsert(ctx context.Context, s *dbsetting.SystemSetting) error {
	return r.with(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value_json", "group", "description", "editable", "updated_at"}),
	}).Create(s).Error
}

func (r *SystemSettingRepository) DeleteByKey(ctx context.Context, key string, soft bool) error {
	db := r.with(ctx).Where("key = ?", key)
	if !soft {
		db = db.Unscoped()
	}
	return db.Delete(&dbsetting.SystemSetting{}).Error
}
