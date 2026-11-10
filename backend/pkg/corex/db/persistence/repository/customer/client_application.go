package customer

import (
	"context"
	"strings"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	base "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ClientApplicationRepository struct {
	*base.BaseRepository[model.ClientApplication]
	db *gorm.DB
}

func NewClientApplicationRepository(db *gorm.DB) *ClientApplicationRepository {
	return &ClientApplicationRepository{BaseRepository: base.NewBaseRepository[model.ClientApplication](db), db: db}
}

func (r *ClientApplicationRepository) List(ctx context.Context, tenant, kind, provider, q string, page, size int) ([]model.ClientApplication, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.ClientApplication{}).Where("tenant_uuid = ?", tenant)
	if kind != "" {
		query = query.Where("kind = ?", kind)
	}
	if q != "" {
		query = query.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(q)+"%")
	}
	if provider != "" {
		bindings := r.db.Model(&model.ClientAppBinding{}).Select("application_uuid").Where("tenant_uuid = ? AND provider = ?", tenant, provider)
		query = query.Where("uuid IN (?)", bindings)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.ClientApplication
	err := query.Order("created_at DESC, id DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error
	return items, total, err
}

func (r *ClientApplicationRepository) Get(ctx context.Context, tenant, id string, lock bool) (model.ClientApplication, error) {
	var app model.ClientApplication
	q := r.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenant, id)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.First(&app).Error
	return app, err
}

func (r *ClientApplicationRepository) Bindings(ctx context.Context, tenant string, ids []string) ([]model.ClientAppBinding, error) {
	var items []model.ClientAppBinding
	if len(ids) == 0 {
		return items, nil
	}
	err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND application_uuid IN (?)", tenant, ids).Order("created_at ASC, id ASC").Find(&items).Error
	return items, err
}

// ResolveRuntime reads authorization revision and its encrypted credential in
// one statement, so a concurrent rotation cannot pair an old secret with a new revision.
func (r *ClientApplicationRepository) ResolveRuntime(ctx context.Context, tenant, platform, appID string) (model.ClientApplication, model.ClientAppBinding, error) {
	var row struct {
		ApplicationUUID           string
		Name, Kind, Status        string
		Version                   int64
		PluginIDs                 datatypes.JSON
		Provider, Platform, AppID string
		SealedSecret              datatypes.JSONMap
	}
	apps, bindings := (model.ClientApplication{}).TableName(), (model.ClientAppBinding{}).TableName()
	err := r.db.WithContext(ctx).Table(apps+" AS a").Select("a.uuid AS application_uuid, a.name, a.kind, a.status, a.version, a.plugin_ids, b.provider, b.platform, b.app_id, b.sealed_secret").
		Joins("JOIN "+bindings+" AS b ON b.application_uuid = a.uuid AND b.tenant_uuid = a.tenant_uuid").
		Where("a.tenant_uuid = ? AND b.platform = ? AND b.app_id = ? AND a.deleted_at IS NULL AND b.deleted_at IS NULL", tenant, platform, appID).Take(&row).Error
	return model.ClientApplication{TenantUUID: tenant, Name: row.Name, Kind: row.Kind, Status: row.Status, Version: row.Version, PluginIDs: row.PluginIDs}, model.ClientAppBinding{TenantUUID: tenant, ApplicationUUID: row.ApplicationUUID, Provider: row.Provider, Platform: row.Platform, AppID: row.AppID, SealedSecret: row.SealedSecret}, err
}

func (r *ClientApplicationRepository) TenantPlugins(ctx context.Context, tenant string) ([]string, error) {
	var plugins []string
	err := r.db.WithContext(ctx).Model(&setting.PluginInstanceConfig{}).Where("tenant_uuid = ? AND key = ? AND enabled = ? AND status = ?", tenant, "auth.credentials", true, setting.PluginInstanceStatusEnabled).Distinct("plugin_id").Order("plugin_id").Pluck("plugin_id", &plugins).Error
	return plugins, err
}

func (r *ClientApplicationRepository) Create(ctx context.Context, app *model.ClientApplication, bindings []model.ClientAppBinding) error {
	if err := r.db.WithContext(ctx).Create(app).Error; err != nil {
		return err
	}
	for i := range bindings {
		bindings[i].ApplicationUUID = app.UUID.String()
		bindings[i].TenantUUID = app.TenantUUID
	}
	return r.db.WithContext(ctx).Create(&bindings).Error
}

func (r *ClientApplicationRepository) Update(ctx context.Context, app model.ClientApplication, fields map[string]any, bindings []model.ClientAppBinding) error {
	result := r.db.WithContext(ctx).Model(&model.ClientApplication{}).Where("tenant_uuid = ? AND uuid = ? AND version = ?", app.TenantUUID, app.UUID, app.Version).Updates(fields)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrAccountProfileChanged
	}
	for _, b := range bindings {
		if err := r.db.WithContext(ctx).Model(&model.ClientAppBinding{}).Where("tenant_uuid = ? AND application_uuid = ? AND uuid = ?", app.TenantUUID, app.UUID, b.UUID).Update("sealed_secret", b.SealedSecret).Error; err != nil {
			return err
		}
	}
	return nil
}
