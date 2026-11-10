package config

import (
	"bytes"
	"fmt"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"path/filepath"
	"regexp"
	"strings"
)

// validateStorageGovernanceYAML 只收紧本模块 schema，兼容项目其他模块现有配置行为。
func validateStorageGovernanceYAML(raw []byte) error {
	var root struct {
		Storage yaml.Node `yaml:"storage_governance"`
	}
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return err
	}
	if root.Storage.Kind == 0 {
		return nil
	}
	if root.Storage.Kind != yaml.MappingNode {
		return fmt.Errorf("storage_governance must be a mapping")
	}
	data, err := yaml.Marshal(&root.Storage)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var cfg StorageGovernanceConfig
	if decoder.Decode(&cfg) != nil {
		return fmt.Errorf("storage_governance contains unsupported fields or field types")
	}
	if cfg.SchemaVersion != 0 && cfg.SchemaVersion != 1 {
		return fmt.Errorf("storage_governance schema_version unsupported")
	}
	if cfg.PostgresClusterID != "" && !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`).MatchString(cfg.PostgresClusterID) {
		return fmt.Errorf("storage_governance postgres_cluster_id invalid")
	}
	if cfg.PostgresEnabled && cfg.PostgresClusterID == "" {
		return fmt.Errorf("storage_governance postgres metrics require registered cluster identity")
	}
	for _, source := range cfg.Sources {
		if source.RotationFormat != "postgresql" {
			continue
		}
		if source.Kind != "postgresql_logs" || !filepath.IsAbs(source.Path) || !filepath.IsAbs(source.CurrentLogfiles) || filepath.Clean(source.CurrentLogfiles) != source.CurrentLogfiles || filepath.Base(source.CurrentLogfiles) != "current_logfiles" || len(source.ActiveFiles) == 0 || len(source.ActiveFiles) > 16 {
			return fmt.Errorf("native PostgreSQL source requires absolute log directory, current_logfiles and filename prefixes")
		}
		for _, prefix := range source.ActiveFiles {
			if prefix == "" || prefix == "." || prefix == ".." || filepath.Base(prefix) != prefix || strings.ContainsAny(prefix, "\x00\r\n") {
				return fmt.Errorf("native PostgreSQL filename prefix invalid")
			}
		}
	}
	if cfg.Mode != "" && cfg.Mode != "observe" {
		return fmt.Errorf("storage_governance mode must remain observe; runtime activation is explicit")
	}
	if cfg.LifecycleEnabled || cfg.AdmissionEnabled {
		if !filepath.IsAbs(cfg.RuntimeDirectory) || cfg.HostID == "" || cfg.FilesystemID == "" {
			return fmt.Errorf("storage governance runtime requires registered host, filesystem and absolute runtime_directory")
		}
	}
	if cfg.ManageNativeRetention && !cfg.LifecycleEnabled {
		return fmt.Errorf("native retention handover requires lifecycle_enabled")
	}
	if cfg.AllowEnforce && !cfg.ManageNativeRetention {
		return fmt.Errorf("enforce requires explicit manage_native_retention handover")
	}
	if cfg.AllowEnforce && !cfg.LifecycleEnabled {
		return fmt.Errorf("storage governance enforce requires lifecycle_enabled")
	}
	if cfg.MinFreeBytes < 0 || cfg.CriticalFreeBytes < 0 || cfg.HostBudgetBytes < 0 || cfg.MaxConcurrent < 0 || cfg.MaxConcurrent > 16 {
		return fmt.Errorf("storage governance budget invalid")
	}
	if cfg.MinFreeBytes > 0 && cfg.CriticalFreeBytes >= cfg.MinFreeBytes {
		return fmt.Errorf("storage governance emergency floor must be below minimum free space")
	}
	if (cfg.NotificationTenantUUID == "") != (cfg.NotificationMemberUUID == "") {
		return fmt.Errorf("storage notification requires an explicit tenant and recipient")
	}
	if cfg.NotificationTenantUUID != "" {
		if _, e := uuid.Parse(cfg.NotificationTenantUUID); e != nil {
			return fmt.Errorf("storage notification tenant invalid")
		}
		if _, e := uuid.Parse(cfg.NotificationMemberUUID); e != nil {
			return fmt.Errorf("storage notification member invalid")
		}
	}
	if cfg.MinFreeBytes > 1<<50 || cfg.HostBudgetBytes > 1<<50 {
		return fmt.Errorf("storage governance budget out of range")
	}
	min := cfg.MinFreeBytes
	if min == 0 {
		min = 20 << 30
	}
	critical := cfg.CriticalFreeBytes
	if critical == 0 {
		critical = 10 << 30
	}
	if (cfg.LifecycleEnabled || cfg.AdmissionEnabled) && critical >= min {
		return fmt.Errorf("storage governance effective floors invalid")
	}
	return nil
}
