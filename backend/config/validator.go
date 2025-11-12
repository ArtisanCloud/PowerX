package config

import (
	"fmt"
	"strings"
	"time"
)

// Validate 验证配置的合法性（已对齐新版 AuthConfig）
func (c *Config) Validate() error {
	var errors []string

	// --- Server ---
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		errors = append(errors, "server.port 必须在 1-65535 范围内")
	}

	// --- Auth ---
	if strings.TrimSpace(c.Auth.JWTSecret) == "" {
		errors = append(errors, "auth.jwt_secret 不能为空")
	} else if len(c.Auth.JWTSecret) < 32 {
		errors = append(errors, "auth.jwt_secret 长度至少32个字符")
	}
	if strings.TrimSpace(c.Auth.Issuer) == "" {
		errors = append(errors, "auth.issuer 不能为空")
	}
	if strings.TrimSpace(c.Auth.AudienceUser) == "" {
		errors = append(errors, "auth.audience_user 不能为空")
	}
	if strings.TrimSpace(c.Auth.AudienceCustomer) == "" {
		errors = append(errors, "auth.audience_customer 不能为空")
	}
	if len(c.Auth.Platforms) == 0 {
		errors = append(errors, "auth.platforms 不能为空")
	}
	// TTL 校验：能解析且 >0
	if d, err := time.ParseDuration(strings.TrimSpace(c.Auth.AccessTTLStr)); err != nil || d <= 0 {
		errors = append(errors, "auth.access_ttl 必须是合法的正 Duration（例如 \"15m\"）")
	}
	if d, err := time.ParseDuration(strings.TrimSpace(c.Auth.RefreshTTLStr)); err != nil || d <= 0 {
		errors = append(errors, "auth.refresh_ttl 必须是合法的正 Duration（例如 \"336h\"）")
	}

	// --- Event Bus ---
	if c.EventBus.Type != "local" && c.EventBus.Type != "redis" {
		errors = append(errors, "event_bus.type 必须是 'local' 或 'redis'")
	}
	if c.EventBus.Type == "redis" && strings.TrimSpace(c.EventBus.RedisAddr) == "" {
		errors = append(errors, "使用 redis 事件总线时，event_bus.redis_addr 不能为空")
	}

	// --- Event Fabric ---
	if c.EventFabric.AckTimeoutSeconds <= 0 {
		errors = append(errors, "event_fabric.ack_timeout_seconds 必须大于0")
	}
	if c.EventFabric.DefaultMaxRetry <= 0 {
		errors = append(errors, "event_fabric.default_max_retry 必须大于0")
	}
	if strings.TrimSpace(c.EventFabric.RetryKeyPrefix) == "" {
		errors = append(errors, "event_fabric.retry_key_prefix 不能为空")
	}
	if strings.TrimSpace(c.EventFabric.ReplayKeyPrefix) == "" {
		errors = append(errors, "event_fabric.replay_key_prefix 不能为空")
	}
	if c.EventFabric.SchedulerInterval <= 0 {
		errors = append(errors, "event_fabric.scheduler_interval 必须大于0")
	}
	if strings.TrimSpace(c.EventFabric.RedisAddr) == "" {
		errors = append(errors, "event_fabric.redis_addr 不能为空")
	}
	if strings.TrimSpace(c.EventFabric.Security.SignatureSecret) != "" {
		if strings.TrimSpace(c.EventFabric.Security.SignatureHeader) == "" {
			errors = append(errors, "event_fabric.security.signature_header 不能为空")
		}
		if strings.TrimSpace(c.EventFabric.Security.TimestampHeader) == "" {
			errors = append(errors, "event_fabric.security.timestamp_header 不能为空")
		}
		if c.EventFabric.Security.AllowedClockSkewSeconds <= 0 {
			errors = append(errors, "event_fabric.security.allowed_clock_skew_seconds 必须大于0")
		}
	}
	if c.EventFabric.Authorization.CacheTTLSeconds <= 0 {
		errors = append(errors, "event_fabric.authorization.cache_ttl_seconds 必须大于0")
	}
	if c.EventFabric.Authorization.LocalCacheTTLSeconds <= 0 {
		errors = append(errors, "event_fabric.authorization.local_cache_ttl_seconds 必须大于0")
	}
	if strings.TrimSpace(c.EventFabric.Authorization.RedisAddr) == "" {
		errors = append(errors, "event_fabric.authorization.redis_addr 不能为空")
	}
	if strings.TrimSpace(c.EventFabric.Authorization.CacheInvalidateChannel) == "" {
		errors = append(errors, "event_fabric.authorization.cache_invalidate_channel 不能为空")
	}
	if c.EventFabric.Authorization.ChallengeSLASeconds <= 0 {
		errors = append(errors, "event_fabric.authorization.challenge_sla_seconds 必须大于0")
	}
	if strings.TrimSpace(c.EventFabric.Authorization.ChallengeTopic) == "" {
		errors = append(errors, "event_fabric.authorization.challenge_topic 不能为空")
	}
	if strings.TrimSpace(c.EventFabric.Authorization.ChallengeConsumerGroup) == "" {
		errors = append(errors, "event_fabric.authorization.challenge_consumer_group 不能为空")
	}
	if c.EventFabric.Authorization.TimeoutSweepIntervalSeconds <= 0 {
		errors = append(errors, "event_fabric.authorization.timeout_sweep_interval_seconds 必须大于0")
	}
	if c.EventFabric.Authorization.AuditRetentionDays <= 0 {
		errors = append(errors, "event_fabric.authorization.audit_retention_days 必须大于0")
	}
	if strings.TrimSpace(c.EventFabric.Authorization.AuditArchiveBucket) == "" {
		errors = append(errors, "event_fabric.authorization.audit_archive_bucket 不能为空")
	}
	if strings.TrimSpace(c.EventFabric.Authorization.AuditArchivePrefix) == "" {
		errors = append(errors, "event_fabric.authorization.audit_archive_prefix 不能为空")
	}
	if c.EventFabric.Authorization.Secrets.CacheTTLSeconds < 0 {
		errors = append(errors, "event_fabric.authorization.secrets.cache_ttl_seconds 不能为负数")
	}
	if c.EventFabric.Authorization.Secrets.RotationIntervalSeconds < 0 {
		errors = append(errors, "event_fabric.authorization.secrets.rotation_interval_seconds 不能为负数")
	}

	// --- LowCode ---
	if c.LowCode.MaxConcurrentFlows <= 0 {
		errors = append(errors, "dynamic_form.max_concurrent_flows 必须大于0")
	}
	if c.LowCode.DefaultTimeoutSec <= 0 {
		errors = append(errors, "dynamic_form.default_timeout_sec 必须大于0")
	}

	// --- Database ---
	if strings.TrimSpace(c.Database.Host) == "" {
		errors = append(errors, "database.host 不能为空")
	}
	if c.Database.Port <= 0 || c.Database.Port > 65535 {
		errors = append(errors, "database.port 必须在 1-65535 范围内")
	}
	if strings.TrimSpace(c.Database.UserName) == "" {
		errors = append(errors, "database.username 不能为空")
	}
	if strings.TrimSpace(c.Database.Database) == "" {
		errors = append(errors, "database.database 不能为空")
	}
	if c.Database.MaxIdleConns < 0 {
		errors = append(errors, "database.max_idle_conns 不能为负数")
	}
	if c.Database.MaxOpenConns <= 0 {
		errors = append(errors, "database.max_open_conns 必须大于0")
	}
	if c.Database.ConnMaxLifetimeMinutes <= 0 {
		errors = append(errors, "database.conn_max_lifetime_minutes 必须大于0")
	}

	// --- Storage ---
	if strings.TrimSpace(c.Storage.DefaultDriver) == "" {
		errors = append(errors, "storage.default_driver 不能为空")
	} else {
		driver := strings.ToLower(strings.TrimSpace(c.Storage.DefaultDriver))
		if driver != "local" && driver != "s3" {
			errors = append(errors, "storage.default_driver 仅支持 local 或 s3")
		}
	}
	if c.Storage.TTLSeconds <= 0 {
		errors = append(errors, "storage.ttl_seconds 必须大于0")
	}
	if strings.EqualFold(c.Storage.DefaultDriver, "local") {
		if strings.TrimSpace(c.Storage.Local.BasePath) == "" {
			errors = append(errors, "storage.local.base_path 不能为空")
		}
	}
	if c.Storage.Local.EnableUploadEndpoint {
		if strings.TrimSpace(c.Storage.Local.UploadTokenSecret) == "" {
			errors = append(errors, "storage.local.upload_token_secret 不能为空（启用 enable_upload_endpoint 时）")
		}
	}
	if strings.EqualFold(c.Storage.DefaultDriver, "s3") {
		if strings.TrimSpace(c.Storage.S3.Endpoint) == "" {
			errors = append(errors, "storage.s3.endpoint 不能为空")
		}
		if strings.TrimSpace(c.Storage.S3.Bucket) == "" {
			errors = append(errors, "storage.s3.bucket 不能为空")
		}
		if strings.TrimSpace(c.Storage.S3.AccessKey) == "" {
			errors = append(errors, "storage.s3.access_key 不能为空")
		}
		if strings.TrimSpace(c.Storage.S3.SecretKey) == "" {
			errors = append(errors, "storage.s3.secret_key 不能为空")
		}
	}

	// --- Plugin Release ---
	if c.PluginRelease.LocalInstall.SessionTTLMinutes <= 0 {
		errors = append(errors, "plugin_release.local_install.session_ttl_minutes 必须大于0")
	}
	if c.PluginRelease.LocalInstall.MaxArtifactSizeMB <= 0 {
		errors = append(errors, "plugin_release.local_install.max_artifact_size_mb 必须大于0")
	}
	if c.PluginRelease.Pipeline.ApprovalSLAHours <= 0 {
		errors = append(errors, "plugin_release.pipeline.approval_sla_hours 必须大于0")
	}
	if c.PluginRelease.Pipeline.MaxParallelReleases <= 0 {
		errors = append(errors, "plugin_release.pipeline.max_parallel_releases 必须大于0")
	}
	if c.PluginRelease.Pipeline.DefaultRollbackNotice <= 0 {
		errors = append(errors, "plugin_release.pipeline.default_rollback_notice_minutes 必须大于0")
	}
	if c.PluginRelease.Canary.RollbackTimeoutSeconds <= 0 {
		errors = append(errors, "plugin_release.canary.rollback_timeout_seconds 必须大于0")
	}
	if c.PluginRelease.Canary.DefaultBatchSize <= 0 {
		errors = append(errors, "plugin_release.canary.default_batch_size 必须大于0")
	}
	if c.PluginRelease.Canary.MaxBatches <= 0 {
		errors = append(errors, "plugin_release.canary.max_batches 必须大于0")
	}
	if strings.TrimSpace(c.PluginRelease.Distribution.OfflineBucket) == "" {
		errors = append(errors, "plugin_release.distribution.offline_bucket 不能为空")
	}
	if strings.TrimSpace(c.PluginRelease.Distribution.OfflinePrefix) == "" {
		errors = append(errors, "plugin_release.distribution.offline_prefix 不能为空")
	}
	if c.PluginRelease.Distribution.EscalationThreshold <= 0 {
		errors = append(errors, "plugin_release.distribution.escalation_threshold 必须大于0")
	}
	if c.PluginRelease.Distribution.ArtifactRetentionDays <= 0 {
		errors = append(errors, "plugin_release.distribution.artifact_retention_days 必须大于0")
	}
	if strings.TrimSpace(c.PluginRelease.Observability.DashboardUID) == "" {
		errors = append(errors, "plugin_release.observability.dashboard_uid 不能为空")
	}
	if strings.TrimSpace(c.PluginRelease.Observability.AlertRulePrefix) == "" {
		errors = append(errors, "plugin_release.observability.alert_rule_prefix 不能为空")
	}
	if c.PluginRelease.Observability.KPITargets.CanRollbackWithinSeconds <= 0 {
		errors = append(errors, "plugin_release.observability.kpi_targets.can_rollback_within_seconds 必须大于0")
	}
	if c.PluginRelease.Observability.KPITargets.HotloadLatencyP95Ms <= 0 {
		errors = append(errors, "plugin_release.observability.kpi_targets.hotload_latency_p95_ms 必须大于0")
	}

	// --- Dev Hotload ---
	if c.DevHotload.Sessions.TTLMinutes <= 0 {
		errors = append(errors, "dev_hotload.sessions.ttl_minutes 必须大于0")
	}
	if c.DevHotload.Sessions.MaxConcurrentSessions <= 0 {
		errors = append(errors, "dev_hotload.sessions.max_concurrent_sessions 必须大于0")
	}
	if c.DevHotload.Sessions.CleanupIntervalSeconds <= 0 {
		errors = append(errors, "dev_hotload.sessions.cleanup_interval_seconds 必须大于0")
	}
	if strings.TrimSpace(c.DevHotload.Sandbox.Image) == "" {
		errors = append(errors, "dev_hotload.sandbox.image 不能为空")
	}
	if c.DevHotload.Sandbox.MaxCPUPercent <= 0 || c.DevHotload.Sandbox.MaxCPUPercent > 100 {
		errors = append(errors, "dev_hotload.sandbox.max_cpu_percent 必须在 1-100 范围内")
	}
	if c.DevHotload.Sandbox.MaxMemoryMB <= 0 {
		errors = append(errors, "dev_hotload.sandbox.max_memory_mb 必须大于0")
	}
	if c.DevHotload.Sandbox.WatchFileLimit <= 0 {
		errors = append(errors, "dev_hotload.sandbox.watch_file_limit 必须大于0")
	}
	if strings.TrimSpace(c.DevHotload.Security.PATHeader) == "" {
		errors = append(errors, "dev_hotload.security.pat_header 不能为空")
	}
	if c.DevHotload.Security.TokenTTLSeconds <= 0 {
		errors = append(errors, "dev_hotload.security.token_ttl_seconds 必须大于0")
	}
	if strings.TrimSpace(c.DevHotload.Observability.MetricsNamespace) == "" {
		errors = append(errors, "dev_hotload.observability.metrics_namespace 不能为空")
	}
	if strings.TrimSpace(c.DevHotload.Observability.AuditTopic) == "" {
		errors = append(errors, "dev_hotload.observability.audit_topic 不能为空")
	}
	if c.DevHotload.Observability.SSEBufferSize <= 0 {
		errors = append(errors, "dev_hotload.observability.sse_buffer_size 必须大于0")
	}

	// --- Plugin Debug ---
	if strings.TrimSpace(c.PluginDebug.Component) == "" {
		errors = append(errors, "plugin_debug.component 不能为空")
	}
	if c.PluginDebug.HostSimulator.Enabled {
		if strings.TrimSpace(c.PluginDebug.HostSimulator.FeatureFlag) == "" {
			errors = append(errors, "plugin_debug.host_simulator.feature_flag 不能为空")
		}
		if strings.TrimSpace(c.PluginDebug.HostSimulator.ConfigPath) == "" {
			errors = append(errors, "plugin_debug.host_simulator.config_path 不能为空")
		}
	}
	if strings.TrimSpace(c.PluginDebug.Reports.TemplatePath) == "" {
		errors = append(errors, "plugin_debug.reports.template 不能为空")
	}
	if strings.TrimSpace(c.PluginDebug.Reports.MaskingRules) == "" {
		errors = append(errors, "plugin_debug.reports.masking_rules 不能为空")
	}
	if strings.TrimSpace(c.PluginDebug.TicketBridge.Provider) == "" {
		errors = append(errors, "plugin_debug.ticket_bridge.provider 不能为空")
	}
	if strings.TrimSpace(c.PluginDebug.TicketBridge.Project) == "" {
		errors = append(errors, "plugin_debug.ticket_bridge.project 不能为空")
	}
	if c.PluginDebug.Sandbox.Enabled {
		if strings.TrimSpace(c.PluginDebug.Sandbox.FeatureFlag) == "" {
			errors = append(errors, "plugin_debug.sandbox.feature_flag 不能为空（启用 sandbox 时）")
		}
		if strings.TrimSpace(c.PluginDebug.Sandbox.DataSuitePath) == "" {
			errors = append(errors, "plugin_debug.sandbox.data_suite_path 不能为空（启用 sandbox 时）")
		}
	}

	// --- Logging ---
	validLevels := []string{"debug", "info", "warn", "error"}
	levelValid := false
	for _, lvl := range validLevels {
		if c.LogConfig.Level == lvl {
			levelValid = true
			break
		}
	}
	if !levelValid {
		errors = append(errors, fmt.Sprintf("logging_config.level 必须是以下之一: %s", strings.Join(validLevels, ", ")))
	}

	if len(errors) > 0 {
		return fmt.Errorf("配置验证失败:\n- %s", strings.Join(errors, "\n- "))
	}
	return nil
}
