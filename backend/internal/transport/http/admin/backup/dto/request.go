package dto

type BackupPolicyUpsertRequest struct {
	Name          string `json:"name" binding:"required,min=1,max=128"`
	BackupType    string `json:"backup_type" binding:"required,min=1,max=32"`
	Schedule      string `json:"schedule" binding:"required,min=1,max=128"`
	RetentionDays int    `json:"retention_days" binding:"required,min=1,max=3650"`
	Enabled       bool   `json:"enabled"`
	StorageTarget string `json:"storage_target" binding:"required,min=1,max=255"`
}

type BackupJobRunRequest struct {
	PolicyID string `json:"policy_id" binding:"required"`
}

type RestoreDrillRunRequest struct {
	SourceJobID string `json:"source_job_id" binding:"required"`
}
