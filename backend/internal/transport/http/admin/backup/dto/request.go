package dto

type BackupPolicyUpsertRequest struct {
	Name              string `json:"name" binding:"required,min=1,max=128"`
	IntervalHours     int    `json:"interval_hours" binding:"omitempty,min=1,max=168"`
	IntervalValue     int    `json:"interval_value" binding:"omitempty,min=1,max=10000"`
	IntervalUnit      string `json:"interval_unit" binding:"omitempty,oneof=minute hour day m h d"`
	Schedule          string `json:"schedule" binding:"omitempty,min=2,max=64"`
	RetentionCount    int    `json:"retention_count" binding:"omitempty,min=1,max=10000"`
	Timezone          string `json:"timezone" binding:"omitempty,min=1,max=64"`
	DrillEnabled      *bool  `json:"drill_enabled"`
	DrillIntervalDays int    `json:"drill_interval_days" binding:"omitempty,min=1,max=3650"`
	TargetRef         string `json:"target_ref" binding:"omitempty,min=1,max=255"`
}

type BackupPolicyUpdateRequest struct {
	Name              *string `json:"name" binding:"omitempty,min=1,max=128"`
	IntervalHours     *int    `json:"interval_hours" binding:"omitempty,min=1,max=168"`
	IntervalValue     *int    `json:"interval_value" binding:"omitempty,min=1,max=10000"`
	IntervalUnit      *string `json:"interval_unit" binding:"omitempty,oneof=minute hour day m h d"`
	Schedule          *string `json:"schedule" binding:"omitempty,min=2,max=64"`
	RetentionCount    *int    `json:"retention_count" binding:"omitempty,min=1,max=10000"`
	Timezone          *string `json:"timezone" binding:"omitempty,min=1,max=64"`
	DrillEnabled      *bool   `json:"drill_enabled"`
	DrillIntervalDays *int    `json:"drill_interval_days" binding:"omitempty,min=1,max=3650"`
	TargetRef         *string `json:"target_ref" binding:"omitempty,min=1,max=255"`
}

type BackupJobRunRequest struct {
	PolicyID string `json:"policy_id" binding:"required"`
}

type RestoreDrillRunRequest struct {
	SourceJobID string `json:"source_job_id,omitempty"`
	ArtifactID  string `json:"artifact_id,omitempty"`
	Reason      string `json:"reason,omitempty" binding:"omitempty,max=255"`
}

type BackupTargetTestRequest struct {
	Driver            string `json:"driver" binding:"omitempty,oneof=postgres"`
	Host              string `json:"host" binding:"required,min=1,max=255"`
	Port              int    `json:"port" binding:"omitempty,min=1,max=65535"`
	Database          string `json:"database" binding:"required,min=1,max=128"`
	Username          string `json:"username" binding:"required,min=1,max=128"`
	Password          string `json:"password" binding:"required,min=1,max=256"`
	SSLMode           string `json:"ssl_mode" binding:"omitempty,oneof=disable require verify-ca verify-full"`
	ConnectTimeoutSec int    `json:"connect_timeout_sec" binding:"omitempty,min=1,max=15"`
}
