package plugininstall

import (
	"context"
	ef "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_install"
	base "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type InstallationRepository struct {
	*base.BaseRepository[m.Installation]
}

func NewInstallationRepository(db *gorm.DB) *InstallationRepository {
	return &InstallationRepository{base.NewBaseRepository[m.Installation](db)}
}
func (r *InstallationRepository) Ready() bool {
	if r == nil || r.DB == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	migrator := r.DB.WithContext(ctx).Migrator()
	return migrator.HasTable(&m.Installation{}) && migrator.HasColumn(&m.Installation{}, "ActorUUID") && migrator.HasColumn(&m.Installation{}, "FailureDetails") && migrator.HasIndex(&m.Installation{}, "uk_plugin_install_request") && migrator.HasTable(&ef.TaskHistory{})
}
func (r *InstallationRepository) Insert(ctx context.Context, tx *gorm.DB, row *m.Installation) (bool, error) {
	if tx == nil {
		tx = r.DB
	}
	out := tx.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_uuid"}, {Name: "actor_id"}, {Name: "request_id"}}, DoNothing: true}).Create(row)
	return out.RowsAffected == 1, out.Error
}
func (r *InstallationRepository) ByRequest(ctx context.Context, tenant string, actor uint64, request string) (*m.Installation, error) {
	var row m.Installation
	e := r.DB.WithContext(ctx).Where("tenant_uuid = ? AND actor_id = ? AND request_id = ?", tenant, actor, request).First(&row).Error
	return &row, e
}
func (r *InstallationRepository) Get(ctx context.Context, tenant, id string) (*m.Installation, error) {
	var row m.Installation
	e := r.DB.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenant, id).First(&row).Error
	return &row, e
}
func (r *InstallationRepository) List(ctx context.Context, tenant string, actor uint64, actorUUID, instance string) ([]m.Installation, error) {
	var rows []m.Installation
	e := r.DB.WithContext(ctx).Where("tenant_uuid = ? AND actor_id = ? AND actor_uuid = ? AND instance_id = ?", tenant, actor, actorUUID, instance).Order("CASE WHEN status IN ('awaiting_upload','queued','running') THEN 0 ELSE 1 END").Order("id DESC").Limit(50).Find(&rows).Error
	return rows, e
}
func (r *InstallationRepository) Update(ctx context.Context, tx *gorm.DB, tenant, id string, apply func(*m.Installation) error) error {
	if tx == nil {
		tx = r.DB
	}
	return tx.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row m.Installation
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_uuid = ? AND uuid = ?", tenant, id).First(&row).Error; e != nil {
			return e
		}
		if e := apply(&row); e != nil {
			return e
		}
		return tx.Save(&row).Error
	})
}
