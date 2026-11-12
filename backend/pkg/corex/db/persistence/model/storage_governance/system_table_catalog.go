package storagegovmodel

// SystemTableDefinition 是平台 Root 只读观测目录，不是可删除白名单。
// 目录不接受 HTTP 提交表名、schema 或时间列；新增表必须先完成用途与保护分类。
type SystemTableDefinition struct {
	Name             string `json:"id"`
	Category         string `json:"category"`
	TimeColumn       string `json:"time_column"`
	CleanupMechanism string `json:"cleanup_mechanism"`
	Protection       string `json:"protection"`
}

// SystemTableCatalog 返回独立副本；受保护数据也可计量，但不提供通用删除能力。
func SystemTableCatalog() []SystemTableDefinition {
	return []SystemTableDefinition{
		{Name: "agent_chat_messages", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "agent_cost_quota_ledgers", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "agent_evidence_snapshots", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "agent_handoff_tasks", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "agent_plan_revisions", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "agent_plan_runs", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "agent_run_observations", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "agent_run_snapshots", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "agent_run_task_states", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "agent_service_invocations", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "agent_service_messages", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "agent_task_events", Category: "runtime", TimeColumn: "ts", CleanupMechanism: "review_required", Protection: "delivery_and_recovery_window"},
		{Name: "agent_verification_evidences", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "ai_usage_logs", Category: "usage", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "settlement_required"},
		{Name: "audit_event", Category: "security", TimeColumn: "occurred_at", CleanupMechanism: "review_required", Protection: "security_policy_required"},
		{Name: "capability_registry_discovery_cache", Category: "observability", TimeColumn: "created_at", CleanupMechanism: "typed_policy", Protection: "retired_cache_only"},
		{Name: "capability_registry_event_publications", Category: "observability", TimeColumn: "created_at", CleanupMechanism: "invocation_policy", Protection: "terminal_and_unreferenced"},
		{Name: "capability_registry_health_probe_results", Category: "observability", TimeColumn: "probe_window_start", CleanupMechanism: "typed_policy", Protection: "keep_latest_probe"},
		{Name: "capability_registry_invocation_traces", Category: "observability", TimeColumn: "created_at", CleanupMechanism: "invocation_policy", Protection: "terminal_and_unreferenced"},
		{Name: "capability_registry_sync_jobs", Category: "observability", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "terminal_and_unreferenced"},
		{Name: "customer_login_events", Category: "security", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "security_policy_required"},
		{Name: "dev_plugin_session_events", Category: "observability", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "terminal_and_unreferenced"},
		{Name: "event_delivery_attempts", Category: "runtime", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "delivery_and_recovery_window"},
		{Name: "event_dlq_messages", Category: "runtime", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "delivery_and_recovery_window"},
		{Name: "event_envelopes", Category: "runtime", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "delivery_and_recovery_window"},
		{Name: "event_replay_requests", Category: "runtime", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "delivery_and_recovery_window"},
		{Name: "event_scheduled_task_runs", Category: "runtime", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "delivery_and_recovery_window"},
		{Name: "event_scheduled_tasks", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "event_subscription_offsets", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "plugin_installations", Category: "runtime", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "delivery_and_recovery_window"},
		{Name: "event_task_histories", Category: "runtime", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "delivery_and_recovery_window"},
		{Name: "iam_authorization_segments", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "iam_authorization_streams", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "iam_authorization_tracking", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "iam_refresh_token", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "iam_registration_policy_audits", Category: "security", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "security_policy_required"},
		{Name: "integration_gateway_api_key_audit_logs", Category: "security", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "security_policy_required"},
		{Name: "integration_gateway_event_publications", Category: "runtime", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "delivery_and_recovery_window"},
		{Name: "integration_gateway_invocation_logs", Category: "observability", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "terminal_and_unreferenced"},
		{Name: "knowledge_audit_trail_entries", Category: "security", TimeColumn: "occurred_at", CleanupMechanism: "review_required", Protection: "security_policy_required"},
		{Name: "knowledge_index_jobs", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "knowledge_ingestion_jobs", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "ops_backup_alerts", Category: "recovery", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "recovery_points_required"},
		{Name: "ops_backup_cleanup_retentions", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "ops_backup_cleanup_resolutions", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "ops_backup_artifacts", Category: "recovery", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "recovery_points_required"},
		{Name: "ops_backup_jobs", Category: "recovery", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "recovery_points_required"},
		{Name: "ops_deploy_release_records", Category: "recovery", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "recovery_points_required"},
		{Name: "ops_migration_runbook_records", Category: "recovery", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "recovery_points_required"},
		{Name: "ops_plugin_lifecycle_audits", Category: "security", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "security_policy_required"},
		{Name: "ops_restore_drill_records", Category: "recovery", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "recovery_points_required"},
		{Name: "ops_storage_cleanup_runs", Category: "observability", TimeColumn: "created_at", CleanupMechanism: "metadata_window", Protection: "metadata_window_only"},
		{Name: TableControlConfiguration, Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: TableControlRevision, Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "ops_storage_policies", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "ops_storage_policy_revisions", Category: "observability", TimeColumn: "created_at", CleanupMechanism: "metadata_window", Protection: "metadata_window_only"},
		{Name: "ops_storage_samples", Category: "observability", TimeColumn: "slot", CleanupMechanism: "metadata_window", Protection: "metadata_window_only"},
		{Name: "plugin_release_local_install_sessions", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "scheduler_job_runs", Category: "runtime", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "delivery_and_recovery_window"},
		{Name: "scheduler_jobs", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "skills_execution_traces", Category: "observability", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "terminal_and_unreferenced"},
		{Name: "skills_lifecycle_audits", Category: "security", TimeColumn: "created_at", CleanupMechanism: "review_required", Protection: "security_policy_required"},
		{Name: "system_notifications", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "workflow_events", Category: "protected", TimeColumn: "occurred_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "workflow_human_review_tasks", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "workflow_instances", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "workflow_step_compensations", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
		{Name: "workflow_step_records", Category: "protected", TimeColumn: "created_at", CleanupMechanism: "protected", Protection: "excluded_from_log_cleanup"},
	}
}
