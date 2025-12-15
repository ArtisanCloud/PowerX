package migration

import (
	"fmt"

	"gorm.io/gorm"
)

// CreatePluginReleaseStatusView builds a materialized view of latest deployment states to power dashboards.
func CreatePluginReleaseStatusView(db *gorm.DB) error {
	if err := db.Exec(`CREATE SCHEMA IF NOT EXISTS plugin_release;`).Error; err != nil {
		return fmt.Errorf("create plugin_release schema failed: %w", err)
	}

	const createSQL = `
CREATE MATERIALIZED VIEW IF NOT EXISTS plugin_release.mv_plugin_release_status AS
SELECT
    prc.tenant_uuid,
    prc.plugin_id,
    prc.version,
    prc.gate_status,
    prc.approval_status,
    rp.status AS plan_status,
    rp.window_start,
    rp.window_end,
    rp.created_at AS plan_created_at,
    rp.updated_at AS plan_updated_at,
    op.status AS offline_package_status,
    ml.review_status AS marketplace_review_status,
    ml.review_count,
    ml.published_at,
    op.sla_deadline
FROM plugin_release_candidates prc
LEFT JOIN plugin_release_plans rp ON rp.release_candidate_id = prc.id
LEFT JOIN plugin_release_offline_packages op ON op.release_candidate_id = prc.id
LEFT JOIN plugin_release_marketplace_listings ml ON ml.offline_package_id = op.id;`

	if err := db.Exec(createSQL).Error; err != nil {
		return fmt.Errorf("create plugin_release status view failed: %w", err)
	}

	const refreshIndex = `
CREATE INDEX IF NOT EXISTS idx_mv_plugin_release_status_tenant_plugin
    ON plugin_release.mv_plugin_release_status (tenant_uuid, plugin_id);`

	return db.Exec(refreshIndex).Error
}
