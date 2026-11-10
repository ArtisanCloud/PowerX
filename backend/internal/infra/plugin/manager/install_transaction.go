package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/runtimecredential"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"gopkg.in/yaml.v3"
)

// InstallFromFile 是所有安装源的共同入口。新包暂存和恢复备份不进入安装扫描目录。
func (m *managerImpl) InstallFromFile(ctx context.Context, src string, opts plugin_mgr.InstallOptions) (result plugin_mgr.Plugin, installErr error) {
	ctx, unlock := m.lockInstallMutation(ctx)
	defer unlock()
	if opts.Origin.Type == "" {
		opts.Origin = plugin_mgr.InstallOrigin{Type: "local", Channel: "directory"}
	}
	opts.Origin = opts.Origin.Normalized()
	man, err := loadManifestWithCatalogs(src)
	if err != nil {
		return result, err
	}
	for _, component := range []string{man.ID, man.Version} {
		if component == "." || component == ".." || strings.ContainsAny(component, "/\\") {
			return result, installConflict("PLUGIN_INVALID_PACKAGE_PATH", "插件 ID 或版本不能包含路径分隔符")
		}
	}
	if err := NewFSLoader().Validate(ctx, man, src); err != nil {
		return result, err
	}
	if err := validateInstallPackagePlatform(man, src); err != nil {
		return result, err
	}
	if _, err := m.requireDeploymentEnv(); err != nil {
		return result, err
	}
	if man.Provider != (plugin_mgr.ProviderIdentity{}) && !man.Provider.Known() {
		return result, installConflict("PLUGIN_PROVIDER_INVALID", "安装包 provider 身份格式无效")
	}
	if opts.Provider.Known() {
		if man.Provider.Known() && !man.Provider.SameProvider(opts.Provider) {
			return result, installConflict("PLUGIN_PROVIDER_MISMATCH", "安装包发布者与市场目录不一致")
		}
		man.Provider = opts.Provider.Normalized()
	} else {
		man.Provider = man.Provider.Normalized()
	}
	old, exists := m.opts.Registry.Get(ctx, man.ID)
	if !exists {
		if opts.ExpectedRevision != "" {
			return result, installConflict("PLUGIN_INSTALL_CHANGED", "原安装已变化，请刷新后重试")
		}
		opts.Force = false
		return m.installDirectory(ctx, src, opts, installRevision(""), nil, nil)
	}
	if opts.ExpectedRevision != "" && old.Revision != opts.ExpectedRevision {
		return result, installConflict("PLUGIN_INSTALL_CHANGED", "当前安装已变化，请刷新后重新确认替换")
	}
	// 原有本地上传以 Root 选择的安装包和 force 开关为准。仅兼容本地及
	// 来源未知的旧安装；市场转本地仍要求显式确认，不降低市场身份核对。
	localUpload := opts.LocalUpload && opts.Origin.Type == "local" && opts.Origin.Channel == "local" && old.InstallOrigin.Normalized().Type != "marketplace"
	if localUpload && !man.Provider.Known() && old.Provider.Known() {
		man.Provider = old.Provider
		opts.Provider = old.Provider
	}
	if old.Provider.Known() && man.Provider.Known() && !old.Provider.SameProvider(man.Provider) {
		return result, installConflict("PLUGIN_PROVIDER_MISMATCH", "相同插件 ID 属于不同发布者，不能替换原插件")
	}
	if localUpload && !opts.Force && old.Version == man.Version {
		if err := m.validatePluginDatabaseBinding(old.ID, old.HostConfig); err != nil {
			return result, err
		}
		if m.opts.PostInstallManifest != nil {
			if err := m.opts.PostInstallManifest(ctx, man); err != nil {
				return result, plugin_mgr.Wrap(plugin_mgr.CodeInternal, err, plugin_mgr.WithOp("install_file.register_permissions"))
			}
		}
		return m.completeExistingInstall(ctx, old, opts)
	}
	// 重复提交相同制品不修改来源和安装修订号。
	if old.Version == man.Version && old.Provider.SameProvider(man.Provider) && opts.Origin.SHA256 != "" && strings.EqualFold(old.InstallOrigin.SHA256, opts.Origin.SHA256) {
		return m.completeExistingInstall(ctx, old, opts)
	}
	if !man.Provider.Known() && !localUpload {
		return result, installConflict("PLUGIN_PROVIDER_UNCONFIRMED", "目标包缺少发布者身份，请补齐 provider 声明后再替换")
	}
	legacyProvider := !old.Provider.Known()
	if !legacyProvider && !old.Provider.SameProvider(man.Provider) {
		return result, installConflict("PLUGIN_PROVIDER_MISMATCH", "相同插件 ID 属于不同发布者，不能替换原插件")
	}
	if localUpload && opts.Force && opts.ExpectedRevision == "" {
		// 在同一个安装锁内读取旧修订号，旧上传客户端不需要增加确认请求。
		opts.ExpectedRevision = old.Revision
	}
	if !opts.Force || opts.ExpectedRevision == "" || (legacyProvider && !opts.ConfirmLegacyProvider && !localUpload) {
		return result, &plugin_mgr.ReplacementRequiredError{PluginID: old.ID, RequiresProviderConfirmation: legacyProvider && !localUpload,
			Current: plugin_mgr.InstallPreview{Version: old.Version, Revision: old.Revision, Provider: old.Provider, Origin: old.InstallOrigin},
			Target:  plugin_mgr.InstallPreview{Version: man.Version, Provider: man.Provider, Origin: opts.Origin},
		}
	}
	if len(m.opts.Registry.ListVersions(ctx, man.ID)) != 1 {
		return result, installConflict("PLUGIN_LEGACY_VERSIONS_PRESENT", "存在历史多版本安装，请先核对并整理，不能自动删除旧版本")
	}
	if m.opts.TenantInstanceCount != nil {
		count, err := m.opts.TenantInstanceCount(ctx, old.ID)
		if err != nil {
			return result, err
		}
		if count > 0 {
			return result, plugin_mgr.NewError(plugin_mgr.CodeConflict, plugin_mgr.WithOp("PLUGIN_DRAIN_REQUIRED"), plugin_mgr.WithPlugin(old.ID), plugin_mgr.WithVersion(old.Version), plugin_mgr.WithMsg("plugin has %d active tenant instances; drain required before replacing enabled runtime", count))
		}
	}
	registry, ok := m.opts.Registry.(*JSONRegistry)
	if !ok {
		return result, installConflict("PLUGIN_REGISTRY_TRANSACTION_REQUIRED", "当前注册表不支持安全替换")
	}
	var credential *PluginRuntimeCredential
	if m.opts.RuntimeCredential != nil {
		credential, err = m.resolvePluginRuntimeCredential(ctx, old.ID)
		if err != nil {
			return result, plugin_mgr.Wrap(plugin_mgr.CodeLifecycleError, err, plugin_mgr.WithOp("install_file.runtime_credential_preflight"))
		}
	}
	if err := m.validatePluginDatabaseBinding(old.ID, old.HostConfig); err != nil {
		return result, err
	}
	targetRoot := filepath.Join(m.opts.InstalledRoot, man.ID, man.Version)
	if targetRoot != old.Paths.Root {
		if _, err := os.Stat(targetRoot); err == nil {
			return result, installConflict("PLUGIN_TARGET_EXISTS", "目标版本目录已有未登记产物，请先核对，不能直接覆盖")
		} else if !os.IsNotExist(err) {
			return result, err
		}
	}
	snapshot, err := registry.installSnapshot(old.ID)
	if err != nil {
		return result, err
	}
	base := filepath.Join(filepath.Dir(m.opts.InstalledRoot), ".powerx-install-backups")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return result, err
	}
	work, err := os.MkdirTemp(base, "replace-")
	if err != nil {
		return result, err
	}
	stage, backup := filepath.Join(work, "target"), filepath.Join(work, "previous")
	if err := copyDir(src, stage); err != nil {
		_ = os.RemoveAll(work)
		return result, err
	}
	defer os.RemoveAll(stage)
	opts.HostConfigSeed = replacementHostSeed(old, opts.HostConfigSeed)
	// 用户配置优先，目标包新增的默认项仍保留；生成和迁移继续由原安装器完成。
	if opts.HostConfigSeed != nil {
		defaults := map[string]any{}
		example := filepath.Join(stage, "config", "values.example.yaml")
		if raw, readErr := os.ReadFile(example); readErr == nil {
			_ = yaml.Unmarshal(raw, &defaults)
		}
		merged := mergeHostSpecMissing(cloneAnyMap(opts.HostConfigSeed.Spec), defaults)
		raw, marshalErr := yaml.Marshal(merged)
		if marshalErr != nil {
			return result, marshalErr
		}
		if err := os.MkdirAll(filepath.Dir(example), 0o755); err != nil {
			return result, err
		}
		if err := runtimecredential.AtomicWrite(example, raw, 0o600); err != nil {
			return result, err
		}
		newRoot := filepath.Join(m.opts.InstalledRoot, man.ID, man.Version)
		for key, value := range opts.HostConfigSeed.Values {
			if strings.HasPrefix(value, old.Paths.Root+string(filepath.Separator)) {
				opts.HostConfigSeed.Values[key] = newRoot + strings.TrimPrefix(value, old.Paths.Root)
			}
		}
	}
	revision := installRevision("")
	journal := installRecovery{PluginID: old.ID, InstalledRoot: m.opts.InstalledRoot, PreviousRoot: old.Paths.Root, BackupRoot: backup, TargetRoot: targetRoot, Revision: revision, Snapshot: snapshot}
	if err := journal.write(work); err != nil {
		return result, err
	}
	defer func() {
		if installErr == nil {
			_ = os.Remove(filepath.Join(work, "recovery.json"))
			return
		}
		recoveryCtx := context.WithoutCancel(ctx)
		var rollbackErr error
		if _, err := os.Stat(backup); err == nil {
			if m.http != nil {
				rollbackErr = m.Disable(recoveryCtx, old.ID)
			}
			if rollbackErr == nil {
				rollbackErr = os.RemoveAll(targetRoot)
			}
			if rollbackErr == nil {
				rollbackErr = os.Rename(backup, old.Paths.Root)
			}
		}
		rollbackErr = errors.Join(rollbackErr, registry.restoreInstallSnapshot(old.ID, snapshot))
		if old.State == plugin_mgr.StateEnabled && m.http != nil {
			_ = registry.UpdateState(recoveryCtx, old.ID, old.Version, plugin_mgr.StateDisabled)
			rollbackErr = errors.Join(rollbackErr, m.Enable(recoveryCtx, old.ID))
		}
		if rollbackErr != nil {
			installErr = errors.Join(installErr, fmt.Errorf("恢复原安装失败: %w", rollbackErr))
		} else {
			_ = os.Remove(filepath.Join(work, "recovery.json"))
		}
	}()
	if old.State == plugin_mgr.StateEnabled {
		if err := m.Disable(ctx, old.ID); err != nil {
			return result, err
		}
	}
	if err := os.Rename(old.Paths.Root, backup); err != nil {
		return result, err
	}
	opts.Force = false
	if !localUpload {
		opts.Metadata = old.InstallMetadata
	}
	result, installErr = m.installDirectory(ctx, stage, opts, revision, credential, func(desc Descriptor) error {
		return registry.commitSingleInstall(desc, old.Revision)
	})
	return result, installErr
}

type installMutationKey struct{}
type installMutationLease struct {
	manager *managerImpl
	active  atomic.Bool
}

func (m *managerImpl) lockInstallMutation(ctx context.Context) (context.Context, func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	if lease, ok := ctx.Value(installMutationKey{}).(*installMutationLease); ok && lease.manager == m && lease.active.Load() {
		return ctx, func() {}
	}
	m.installMu.Lock()
	lease := &installMutationLease{manager: m}
	lease.active.Store(true)
	return context.WithValue(ctx, installMutationKey{}, lease), func() { lease.active.Store(false); m.installMu.Unlock() }
}

type installRecovery struct {
	PluginID      string          `json:"plugin_id"`
	InstalledRoot string          `json:"installed_root"`
	PreviousRoot  string          `json:"previous_root"`
	BackupRoot    string          `json:"backup_root"`
	TargetRoot    string          `json:"target_root"`
	Revision      string          `json:"revision"`
	Snapshot      regPluginRecord `json:"snapshot"`
}

func (r installRecovery) write(work string) error {
	raw, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return runtimecredential.AtomicWrite(filepath.Join(work, "recovery.json"), raw, 0o600)
}

// recoverPendingInstalls 在扫描产物前恢复中断的替换，避免将备份或半成品登记为安装。
func (m *managerImpl) recoverPendingInstalls(ctx context.Context) error {
	r, ok := m.opts.Registry.(*JSONRegistry)
	if !ok {
		return nil
	}
	base := filepath.Join(filepath.Dir(m.opts.InstalledRoot), ".powerx-install-backups")
	files, err := filepath.Glob(filepath.Join(base, "replace-*", "recovery.json"))
	if err != nil {
		return err
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		var journal installRecovery
		if err := json.Unmarshal(raw, &journal); err != nil {
			return err
		}
		if filepath.Clean(journal.InstalledRoot) != filepath.Clean(m.opts.InstalledRoot) {
			continue
		}
		pluginRoot := filepath.Join(m.opts.InstalledRoot, journal.PluginID)
		validSnapshot := len(journal.Snapshot.Versions) == 1
		for version, previous := range journal.Snapshot.Versions {
			validSnapshot = validSnapshot && previous.Manifest.ID == journal.PluginID && previous.Manifest.Version == version && previous.Paths.Root == journal.PreviousRoot && (journal.Snapshot.Current == "" || journal.Snapshot.Current == version)
		}
		if journal.Revision == "" || !validSnapshot || !insideInstallPath(m.opts.InstalledRoot, pluginRoot) || !insideInstallPath(pluginRoot, journal.PreviousRoot) || !insideInstallPath(pluginRoot, journal.TargetRoot) || filepath.Clean(journal.BackupRoot) != filepath.Join(filepath.Dir(file), "previous") || !insideInstallPath(base, journal.BackupRoot) || journal.PluginID == "" {
			return fmt.Errorf("invalid plugin install recovery journal")
		}
		if current, ok := r.Get(ctx, journal.PluginID); ok && current.Revision == journal.Revision {
			if _, err := os.Stat(journal.TargetRoot); err == nil {
				_ = os.Remove(file)
				continue
			}
		}
		if _, err := os.Stat(journal.BackupRoot); err == nil {
			if err := os.RemoveAll(journal.TargetRoot); err != nil {
				return err
			}
			if err := os.Rename(journal.BackupRoot, journal.PreviousRoot); err != nil {
				return err
			}
		} else if _, oldErr := os.Stat(journal.PreviousRoot); oldErr != nil {
			return fmt.Errorf("plugin install recovery requires original artifacts")
		}
		if err := r.restoreInstallSnapshot(journal.PluginID, journal.Snapshot); err != nil {
			return err
		}
		if err := os.Remove(file); err != nil {
			return err
		}
	}
	return nil
}

func insideInstallPath(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func installConflict(code, message string) error {
	return plugin_mgr.NewError(plugin_mgr.CodeConflict, plugin_mgr.WithOp(code), plugin_mgr.WithMsg("%s", message))
}

func replacementHostSeed(old plugin_mgr.Plugin, requested *plugin_mgr.HostConfig) *plugin_mgr.HostConfig {
	seed := cloneHostConfig(old.HostConfig)
	if loaded, err := loadHostConfig(old.Paths.HostValuesFile); err == nil {
		seed = loaded
	}
	if seed == nil {
		seed = &plugin_mgr.HostConfig{Values: map[string]string{}, Spec: map[string]any{}}
	}
	if requested != nil {
		seed.Values = mergeStringMapOverride(seed.Values, requested.Values)
		seed.Spec = mergeHostSpecMissing(cloneAnyMap(seed.Spec), requested.Spec)
	}
	return seed
}

func (r *JSONRegistry) installSnapshot(id string) (regPluginRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var snapshot regPluginRecord
	raw, err := json.Marshal(r.mem.Plugins[id])
	if err == nil {
		err = json.Unmarshal(raw, &snapshot)
	}
	return snapshot, err
}

func (r *JSONRegistry) writeInstallLocked() error {
	raw, err := json.MarshalIndent(r.mem, "", "  ")
	if err != nil {
		return err
	}
	return runtimecredential.AtomicWrite(r.path, raw, 0o600)
}

func (r *JSONRegistry) restoreInstallSnapshot(id string, snapshot regPluginRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mem.Plugins[id] = snapshot
	return r.writeInstallLocked()
}

func (r *JSONRegistry) commitSingleInstall(desc Descriptor, expectedRevision string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, version := desc.Manifest.ID, desc.Manifest.Version
	before := r.mem.Plugins[id]
	if len(before.Versions) != 1 {
		return installConflict("PLUGIN_INSTALL_CHANGED", "原安装已变化，请刷新后重新确认替换")
	}
	for _, old := range before.Versions {
		if old.Revision != expectedRevision {
			return installConflict("PLUGIN_INSTALL_CHANGED", "原安装已变化，请刷新后重新确认替换")
		}
	}
	r.mem.Plugins[id] = regPluginRecord{Current: version, Versions: map[string]regVersionRecord{version: {
		Revision: desc.Revision, State: plugin_mgr.StateInstalled, InstalledAt: time.Now().UTC(), Manifest: desc.Manifest,
		Paths: desc.Paths, HostConfig: cloneHostConfig(desc.HostConfig), Migrations: cloneMigrationRecord(desc.Migration), InstallMetadata: desc.InstallMetadata, InstallOrigin: desc.InstallOrigin.Normalized(),
	}}}
	if err := r.writeInstallLocked(); err != nil {
		r.mem.Plugins[id] = before
		return err
	}
	return nil
}

func (m *managerImpl) completeExistingInstall(ctx context.Context, old plugin_mgr.Plugin, opts plugin_mgr.InstallOptions) (plugin_mgr.Plugin, error) {
	if opts.AutoEnable && old.State != plugin_mgr.StateEnabled {
		return m.SwitchVersion(ctx, old.ID, old.Version, true)
	}
	return old, nil
}
