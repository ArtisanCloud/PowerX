package ops

// BackupPolicyUpsertRequest 为备份策略新增/更新请求 DTO 骨架。
type BackupPolicyUpsertRequest struct {
	Name          string `json:"name"`
	BackupType    string `json:"backup_type"`
	Schedule      string `json:"schedule"`
	RetentionDays int    `json:"retention_days"`
	Enabled       bool   `json:"enabled"`
	StorageTarget string `json:"storage_target"`
}

// BackupJobRunRequest 为手动触发备份任务请求 DTO 骨架。
type BackupJobRunRequest struct {
	PolicyID string `json:"policy_id"`
}

// RestoreDrillRunRequest 为恢复演练触发请求 DTO 骨架。
type RestoreDrillRunRequest struct {
	SourceJobID string `json:"source_job_id"`
}
