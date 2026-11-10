package config

// StorageGovernanceConfig 只允许观察模式；未来写策略不能通过配置偷偷启用。
type StorageGovernanceConfig struct {
	// 高级远端归档和旧执行器接管的部署准入；默认关闭，不代替 UI 策略和现场验收。
	AdvancedOperationsEnabled bool                    `yaml:"advanced_operations_enabled,omitempty" json:"advanced_operations_enabled"`
	Mode                      string                  `yaml:"mode,omitempty" json:"mode,omitempty"`
	HostID                    string                  `yaml:"host_id,omitempty" json:"host_id,omitempty"`
	FilesystemID              string                  `yaml:"filesystem_id,omitempty" json:"filesystem_id,omitempty"`
	SchemaVersion             int                     `yaml:"schema_version,omitempty" json:"schema_version,omitempty"`
	SamplingEnabled           bool                    `yaml:"sampling_enabled,omitempty" json:"sampling_enabled,omitempty"`
	PostgresEnabled           bool                    `yaml:"postgres_enabled,omitempty" json:"postgres_enabled,omitempty"`
	PostgresClusterID         string                  `yaml:"postgres_cluster_id,omitempty" json:"postgres_cluster_id,omitempty"`
	FilesystemPath            string                  `yaml:"filesystem_path,omitempty" json:"-"`
	Sources                   []StorageObserverSource `yaml:"sources,omitempty" json:"-"`
	RuntimeDirectory          string                  `yaml:"runtime_directory,omitempty" json:"-"`
	LifecycleEnabled          bool                    `yaml:"lifecycle_enabled,omitempty" json:"lifecycle_enabled"`
	ManageNativeRetention     bool                    `yaml:"manage_native_retention,omitempty" json:"manage_native_retention"`
	AllowEnforce              bool                    `yaml:"allow_enforce,omitempty" json:"allow_enforce"`
	AdmissionEnabled          bool                    `yaml:"admission_enabled,omitempty" json:"admission_enabled"`
	MinFreeBytes              int64                   `yaml:"min_free_bytes,omitempty" json:"min_free_bytes"`
	CriticalFreeBytes         int64                   `yaml:"critical_free_bytes,omitempty" json:"critical_free_bytes"`
	HostBudgetBytes           int64                   `yaml:"host_budget_bytes,omitempty" json:"host_budget_bytes"`
	MaxConcurrent             int                     `yaml:"max_concurrent,omitempty" json:"max_concurrent"`
	NotificationTenantUUID    string                  `yaml:"notification_tenant_uuid,omitempty" json:"-"`
	NotificationMemberUUID    string                  `yaml:"notification_member_uuid,omitempty" json:"-"`
}

// StorageObserverSource 仅由受信任部署文件登记；API 不接受目录和实例覆盖。
type StorageObserverSource struct {
	ID                     string   `yaml:"id"`
	Name                   string   `yaml:"name"`
	Kind                   string   `yaml:"kind"`
	Scope                  string   `yaml:"scope"`
	InstanceID             string   `yaml:"instance_id,omitempty"`
	Path                   string   `yaml:"path"`
	TenantUUID             string   `yaml:"tenant_uuid,omitempty"`
	ActiveFiles            []string `yaml:"active_files,omitempty"`
	RotationFormat         string   `yaml:"rotation_format,omitempty"`
	CurrentLogfiles        string   `yaml:"current_logfiles,omitempty"`
	FileOwnerUID           *uint32  `yaml:"file_owner_uid,omitempty"`
	RequireVerifiedArchive bool     `yaml:"require_verified_archive,omitempty"`
}
