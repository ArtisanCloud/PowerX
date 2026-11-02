package config

import (
	"strings"
	"testing"
)

func TestStorageConfigRejectsUnknownFields(t *testing.T) {
	for _, input := range []string{
		"storage_governance: {schema_version: 2}",
		"storage_governance: {cleanup_enabled: true}",
		"storage_governance: {sampling_enabled: nonsense}",
		"storage_governance: {sources: [{id: logs, credential: secret}]}",
		"storage_governance: []",
	} {
		if err := validateStorageGovernanceYAML([]byte(input)); err == nil {
			t.Fatalf("accepted unsupported config: %s", input)
		}
	}
	for _, input := range []string{"other_module: {unknown: accepted}", "storage_governance: {schema_version: 1, mode: observe, sampling_enabled: false}"} {
		if err := validateStorageGovernanceYAML([]byte(input)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNativePostgresRegistrationRequiresExplicitTrustedManifest(t *testing.T) {
	valid := `storage_governance: {sources: [{id: pg, name: PostgreSQL, kind: postgresql_logs, scope: shared, path: /pg/log, rotation_format: postgresql, active_files: [postgresql-], current_logfiles: /pg/current_logfiles}]}`
	if err := validateStorageGovernanceYAML([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		strings.Replace(valid, "current_logfiles: /pg/current_logfiles", "current_logfiles: relative/current_logfiles", 1),
		strings.Replace(valid, ", current_logfiles: /pg/current_logfiles", "", 1),
		strings.Replace(valid, "postgresql_logs", "application_logs", 1),
		strings.Replace(valid, "active_files: [postgresql-]", "active_files: [../postgresql-]", 1),
		strings.Replace(valid, "path: /pg/log", "path: relative/log", 1),
	} {
		if validateStorageGovernanceYAML([]byte(input)) == nil {
			t.Fatalf("accepted invalid native log config: %s", input)
		}
	}
}

func TestStorageRuntimeRequiresExplicitHostAndRecipientAndEffectiveFloors(t *testing.T) {
	for _, input := range []string{
		`storage_governance: {lifecycle_enabled: true}`,
		`storage_governance: {allow_enforce: true}`,
		`storage_governance: {admission_enabled: true, runtime_directory: /tmp/state, host_id: h, filesystem_id: f, min_free_bytes: 100}`,
		`storage_governance: {notification_tenant_uuid: invalid, notification_member_uuid: invalid}`,
	} {
		if validateStorageGovernanceYAML([]byte(input)) == nil {
			t.Fatalf("accepted %s", input)
		}
	}
	if err := validateStorageGovernanceYAML([]byte(`storage_governance: {lifecycle_enabled: true, runtime_directory: /var/lib/powerx/storage-runtime, host_id: host, filesystem_id: disk, min_free_bytes: 21474836480, critical_free_bytes: 10737418240}`)); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{`log: {loki: {queue_capacity: -1}}`, `log: {loki: {max_entry_bytes: 100000, max_queue_bytes: 1000}}`, `log: {loki: {request_timeout_ms: 1000000}}`} {
		if validateLokiYAML([]byte(input)) == nil {
			t.Fatalf("accepted %s", input)
		}
	}
}

func TestStorageNativeRetentionHandoverPreservesRotationAndOriginalConfiguration(t *testing.T) {
	cfg := &Config{}
	cfg.LogConfig.File.MaxAge = 30
	cfg.LogConfig.File.MaxBackups = 5
	cfg.LogConfig.File.MaxSize = 100
	cfg.LogConfig.File.Compress = true
	cfg.StorageGovernance = StorageGovernanceConfig{LifecycleEnabled: true, ManageNativeRetention: true}
	actual := cfg.EffectiveLogConfig()
	if actual.File.MaxAge != 0 || actual.File.MaxBackups != 0 || actual.File.MaxSize != 100 || !actual.File.Compress {
		t.Fatal("handover must stop destructive native retention while preserving rotation and compression")
	}
	if cfg.LogConfig.File.MaxAge != 30 || cfg.LogConfig.File.MaxBackups != 5 {
		t.Fatal("source config was silently changed")
	}
	if err := validateStorageGovernanceYAML([]byte(`storage_governance: {allow_enforce: true, lifecycle_enabled: true, runtime_directory: /var/lib/powerx/storage-runtime, host_id: h, filesystem_id: f}`)); err == nil {
		t.Fatal("enforcement without explicit native handover was accepted")
	}
}
