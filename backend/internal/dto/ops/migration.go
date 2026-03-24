package ops

// MigrationRunRequest 为实例迁移执行请求 DTO 骨架。
type MigrationRunRequest struct {
	SourceEnv string `json:"source_env"`
	TargetEnv string `json:"target_env"`
	DryRun    bool   `json:"dry_run"`
}

// MigrationSwitchRequest 为流量切换请求 DTO 骨架。
type MigrationSwitchRequest struct {
	MigrationID string `json:"migration_id"`
	Rollback    bool   `json:"rollback"`
}

// MigrationAcceptanceRequest 为迁移验收请求 DTO。
type MigrationAcceptanceRequest struct {
	DBMigrationCompleted    bool   `json:"db_migration_completed"`
	InstanceMigrationPassed bool   `json:"instance_migration_passed"`
	Conclusion              string `json:"conclusion"`
}
