package migration

import "gorm.io/gorm"

// EnsurePluginReleaseCandidateUniqueIndex creates the unique index required for upserts.
func EnsurePluginReleaseCandidateUniqueIndex(db *gorm.DB) error {
	const sql = `CREATE UNIQUE INDEX IF NOT EXISTS idx_plugin_release_candidate_tenant_plugin_version ON plugin_release_candidates(tenant_id, plugin_id, version);`
	return db.Exec(sql).Error
}
