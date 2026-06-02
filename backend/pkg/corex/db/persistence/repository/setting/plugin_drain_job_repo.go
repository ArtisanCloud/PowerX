package setting

import (
	"context"
	"strings"
	"time"

	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gorm.io/gorm"
)

type PluginDrainJobRepository struct{ db *gorm.DB }

func NewPluginDrainJobRepository(db *gorm.DB) *PluginDrainJobRepository {
	return &PluginDrainJobRepository{db: db}
}

func (r *PluginDrainJobRepository) with(ctx context.Context) *gorm.DB {
	db := r.db.WithContext(ctx)
	if debug, ok := ctx.Value(utils.DebugKey).(bool); ok && debug {
		db = db.Debug()
	}
	return db
}

func (r *PluginDrainJobRepository) Create(ctx context.Context, job *dbsetting.PluginDrainJob) error {
	return r.with(ctx).Create(job).Error
}

func (r *PluginDrainJobRepository) GetByJobID(ctx context.Context, jobID string) (*dbsetting.PluginDrainJob, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var row dbsetting.PluginDrainJob
	err := r.with(ctx).Where("job_id = ?", jobID).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *PluginDrainJobRepository) Update(ctx context.Context, job *dbsetting.PluginDrainJob) error {
	return r.with(ctx).Save(job).Error
}

func (r *PluginDrainJobRepository) ListByPlugin(ctx context.Context, pluginID string, limit int) ([]*dbsetting.PluginDrainJob, error) {
	pluginID = strings.TrimSpace(pluginID)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows []*dbsetting.PluginDrainJob
	err := r.with(ctx).
		Where("plugin_id = ?", pluginID).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func (r *PluginDrainJobRepository) CountBlockingByPlugin(ctx context.Context, pluginID string) (int64, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return 0, nil
	}
	var count int64
	err := r.with(ctx).
		Model(&dbsetting.PluginDrainJob{}).
		Where("plugin_id = ? AND status IN ?", pluginID, []string{
			dbsetting.PluginDrainJobStatusRequested,
			dbsetting.PluginDrainJobStatusBlockingNewUsage,
			dbsetting.PluginDrainJobStatusDraining,
			dbsetting.PluginDrainJobStatusReadyToUninstall,
		}).
		Count(&count).Error
	return count, err
}

func (r *PluginDrainJobRepository) MarkReadyJobsCompletedByPlugin(ctx context.Context, pluginID string, now time.Time) (int64, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return 0, nil
	}
	res := r.with(ctx).
		Model(&dbsetting.PluginDrainJob{}).
		Where("plugin_id = ? AND status = ?", pluginID, dbsetting.PluginDrainJobStatusReadyToUninstall).
		Updates(map[string]any{
			"status":       dbsetting.PluginDrainJobStatusCompleted,
			"completed_at": now,
			"updated_at":   now,
		})
	return res.RowsAffected, res.Error
}
