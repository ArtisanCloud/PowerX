package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/runtimecredential"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/ArtisanCloud/PowerX/pkg/redisnamespace"
	"gopkg.in/yaml.v3"
)

// LegacyLicenseCachePreview 只显示范围和指纹，不包含配置、Redis URL 或任何凭据原文。
// 匹配旧生成格式只是候选依据，不代表已证明来源；apply 必须由操作者确认。
type LegacyLicenseCachePreview struct {
	PluginID             string   `json:"plugin_id"`
	Version              string   `json:"version"`
	RegistryPath         string   `json:"registry_path"`
	ValuesPath           string   `json:"values_path"`
	Status               string   `json:"status"`
	Reason               string   `json:"reason"`
	CacheFields          []string `json:"cache_fields"`
	RequiresConfirmation bool     `json:"requires_confirmation"`
	PlanDigest           string   `json:"plan_digest"`
}

type legacyCacheSnapshot struct {
	report                 LegacyLicenseCachePreview
	registry               *JSONRegistry
	plugin                 plugin_mgr.Plugin
	registryRaw, valuesRaw []byte
	doc                    map[string]any
}

// PreviewLegacyLicenseCache 不触发 Bootstrap、插件发现、数据库连接或运行时资源准备。
func PreviewLegacyLicenseCache(ctx context.Context, registryPath, pluginID, version string) (LegacyLicenseCachePreview, error) {
	s, err := readLegacyCacheSnapshot(ctx, registryPath, pluginID, version)
	if err != nil {
		return LegacyLicenseCachePreview{}, err
	}
	return s.report, nil
}

func readLegacyCacheSnapshot(ctx context.Context, registryPath, pluginID, version string) (*legacyCacheSnapshot, error) {
	path, err := filepath.Abs(registryPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("PLUGIN_REGISTRY_NOT_REGULAR")
	}
	registryRaw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	reg := NewJSONRegistry(path)
	if err := reg.Load(ctx); err != nil {
		return nil, err
	}
	var p plugin_mgr.Plugin
	var ok bool
	if version == "" {
		p, ok = reg.Get(ctx, pluginID)
	} else {
		p, ok = reg.GetVersion(ctx, pluginID, version)
	}
	if !ok || p.HostConfig == nil {
		return nil, fmt.Errorf("PLUGIN_HOST_CONFIG_NOT_FOUND")
	}
	valuesPath, err := filepath.Abs(p.Paths.HostValuesFile)
	if err != nil {
		return nil, err
	}
	if p.Paths.HostValuesFile == "" {
		return nil, fmt.Errorf("PLUGIN_HOST_VALUES_REQUIRED")
	}
	root, err := filepath.EvalSymlinks(p.Paths.Root)
	if err != nil {
		return nil, err
	}
	realValues, err := filepath.EvalSymlinks(valuesPath)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, realValues)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("PLUGIN_HOST_VALUES_OUTSIDE_PACKAGE")
	}
	// 清理路径本身必须为实际文件，避免修改链接后的目标或在替换时改变链接语义。
	stat, err := os.Lstat(valuesPath)
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() {
		return nil, fmt.Errorf("PLUGIN_HOST_VALUES_NOT_REGULAR")
	}
	raw, err := os.ReadFile(valuesPath)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil || doc == nil {
		return nil, fmt.Errorf("PLUGIN_HOST_VALUES_INVALID")
	}
	report := LegacyLicenseCachePreview{PluginID: p.ID, Version: p.Version, RegistryPath: path, ValuesPath: valuesPath}
	fileStatus, fields := legacyLicenseCacheStatus(doc, p.ID)
	memoryStatus, _ := legacyLicenseCacheStatus(p.HostConfig.Spec, p.ID)
	report.CacheFields = fields
	a, _ := json.Marshal(stripHostConfigMeta(cloneAnyMap(doc)))
	b, _ := json.Marshal(p.HostConfig.Spec)
	switch {
	case string(a) != string(b):
		report.Status = "configuration_drift"
		report.Reason = "host-values 与 registry 的 Spec 不一致，需要先核对并同步，不能直接清理"
	case fileStatus == "absent" && memoryStatus == "absent":
		report.Status = "absent"
		report.Reason = "没有旧 License cache 配置"
	case fileStatus == "candidate" && memoryStatus == "candidate":
		report.Status = "legacy_default_candidate"
		report.RequiresConfirmation = true
		report.Reason = "仅匹配旧 Core 默认注入格式；须确认由旧注入生成，默认不修改"
	default:
		report.Status = "manual_migration_required"
		report.Reason = "含人工配置、未知值或空 cache 区块，需按插件 Framework Cache 合同迁移；工具不会删除"
	}
	h := sha256.New()
	h.Write(registryRaw)
	h.Write([]byte{0})
	h.Write(raw)
	h.Write([]byte{0})
	h.Write([]byte(p.ID + "\x00" + p.Version + "\x00" + valuesPath))
	report.PlanDigest = hex.EncodeToString(h.Sum(nil))
	return &legacyCacheSnapshot{report: report, registry: reg, plugin: p, registryRaw: registryRaw, valuesRaw: raw, doc: doc}, nil
}

func legacyLicenseCacheStatus(doc map[string]any, pluginID string) (string, []string) {
	marketplace, _ := doc["marketplace"].(map[string]any)
	license, _ := marketplace["license"].(map[string]any)
	raw, exists := license["cache"]
	if !exists || raw == nil {
		return "absent", nil
	}
	cache, ok := raw.(map[string]any)
	if !ok {
		return "manual", nil
	}
	fields := make([]string, 0, len(cache))
	for key := range cache {
		fields = append(fields, key)
	}
	sort.Strings(fields)
	if len(cache) != 1 {
		return "manual", fields
	}
	prefix, ok := cache["key_prefix"].(string)
	if !ok {
		return "manual", fields
	}
	host, _ := doc["host"].(map[string]any)
	redis, _ := host["redis"].(map[string]any)
	namespace, _ := host["environment_id"].(string)
	if namespace == "" || namespace != strings.TrimSpace(namespace) {
		return "manual", fields
	}
	expected := redisnamespace.Key(namespace, "plugin:"+pluginID+":")
	if redis["key_prefix"] != expected || prefix != expected+"license" {
		return "manual", fields
	}
	if env, ok := doc["env"].(map[string]any); ok {
		if v, exists := env["POWERX_ENVIRONMENT_ID"]; exists && v != namespace {
			return "manual", fields
		}
		if v, exists := env["POWERX_REDIS_KEY_PREFIX"]; exists && v != expected {
			return "manual", fields
		}
	}
	return "candidate", fields
}

func removeLegacyLicenseCache(doc map[string]any) {
	marketplace, _ := doc["marketplace"].(map[string]any)
	license, _ := marketplace["license"].(map[string]any)
	delete(license, "cache")
	if len(license) == 0 {
		delete(marketplace, "license")
	}
	if len(marketplace) == 0 {
		delete(doc, "marketplace")
	}
}

// RepairLegacyLicenseCache 仅在停服窗口、确认来源且预览指纹未变时清理候选字段。
// backupDir 必须是新目录，内含 0600 原文件；失败时调用公共配置提交回滚，崩溃可用备份恢复。
func RepairLegacyLicenseCache(ctx context.Context, registryPath, pluginID, version, digest, backupDir string, confirmed, coreStopped bool) (LegacyLicenseCachePreview, error) {
	if !confirmed || !coreStopped {
		return LegacyLicenseCachePreview{}, fmt.Errorf("PLUGIN_HOST_CONFIG_CONFIRMATION_REQUIRED: require confirmed legacy injection and stopped Core")
	}
	s, err := readLegacyCacheSnapshot(ctx, registryPath, pluginID, version)
	if err != nil {
		return LegacyLicenseCachePreview{}, err
	}
	if digest == "" || s.report.PlanDigest != digest {
		return s.report, fmt.Errorf("PLUGIN_HOST_CONFIG_PREVIEW_STALE")
	}
	if s.report.Status != "legacy_default_candidate" {
		return s.report, fmt.Errorf("PLUGIN_HOST_CONFIG_MANUAL_MIGRATION_REQUIRED")
	}
	if backupDir == "" {
		return s.report, fmt.Errorf("PLUGIN_HOST_CONFIG_BACKUP_REQUIRED")
	}
	backupDir, err = filepath.Abs(backupDir)
	if err != nil {
		return s.report, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(backupDir))
	if err != nil {
		return s.report, err
	}
	backupDir = filepath.Join(parent, filepath.Base(backupDir))
	packageRoot, err := filepath.EvalSymlinks(s.plugin.Paths.Root)
	if err != nil {
		return s.report, err
	}
	inside, err := filepath.Rel(packageRoot, backupDir)
	if err != nil {
		return s.report, err
	}
	if inside != ".." && !strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return s.report, fmt.Errorf("PLUGIN_HOST_CONFIG_BACKUP_INSIDE_PACKAGE")
	}
	if err := os.Mkdir(backupDir, 0700); err != nil {
		return s.report, fmt.Errorf("PLUGIN_HOST_CONFIG_BACKUP_FAILED: %w", err)
	}
	for name, raw := range map[string][]byte{"registry.json": s.registryRaw, "host-values.yaml": s.valuesRaw} {
		if err := runtimecredential.AtomicWrite(filepath.Join(backupDir, name), raw, 0600); err != nil {
			return s.report, err
		}
	}
	metadata, _ := json.Marshal(s.report)
	if err := runtimecredential.AtomicWrite(filepath.Join(backupDir, "preview.json"), metadata, 0600); err != nil {
		return s.report, err
	}
	// 备份期间发生修改也不能继续使用旧计划；离线标记是操作者声明，不会停止服务。
	current, err := readLegacyCacheSnapshot(ctx, registryPath, pluginID, version)
	if err != nil {
		return s.report, err
	}
	if current.report.PlanDigest != digest {
		return current.report, fmt.Errorf("PLUGIN_HOST_CONFIG_PREVIEW_STALE")
	}
	removeLegacyLicenseCache(s.doc)
	data, err := yaml.Marshal(s.doc)
	if err != nil {
		return s.report, err
	}
	next := cloneHostConfig(s.plugin.HostConfig)
	next.Spec = stripHostConfigMeta(cloneAnyMap(s.doc))
	m := &managerImpl{opts: Options{Registry: s.registry}}
	if err := m.commitHostConfigUpdate(ctx, &s.plugin, next, data, true); err != nil {
		return s.report, err
	}
	result, err := PreviewLegacyLicenseCache(ctx, registryPath, pluginID, version)
	return result, err
}
