package migration

import "gorm.io/gorm"

// EnsurePluginReleaseActorToken adds actor_token column for auditing truncated tokens.
func EnsurePluginReleaseActorToken(db *gorm.DB) error {
	const sql = `ALTER TABLE "public"."plugin_release_candidates" ADD COLUMN IF NOT EXISTS actor_token VARCHAR(256)`
	return db.Exec(sql).Error
}
