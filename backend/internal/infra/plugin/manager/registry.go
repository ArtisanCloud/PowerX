package manager

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

type Registry interface {
	Load(ctx context.Context) error
	Save(ctx context.Context) error

	Put(ctx context.Context, desc Descriptor, state plugin_mgr.PluginState) error
	UpdateState(ctx context.Context, id, version string, state plugin_mgr.PluginState) error
	Remove(ctx context.Context, id, version string) error

	Get(ctx context.Context, id string) (plugin_mgr.Plugin, bool)
	List(ctx context.Context) []plugin_mgr.Plugin
	GetVersion(ctx context.Context, id, version string) (plugin_mgr.Plugin, bool)
	CurrentVersion(ctx context.Context, id string) (string, bool)
	// 判断版本是否已安装
	HasVersion(ctx context.Context, id, version string) bool
	// 设置某插件的 current 指针
	SetCurrent(ctx context.Context, id, version string) error
}

type regVersionRecord struct {
	State       plugin_mgr.PluginState      `json:"state"`
	InstalledAt time.Time                   `json:"installed_at"`
	Manifest    plugin_mgr.Manifest         `json:"manifest"`
	Paths       plugin_mgr.InstalledPaths   `json:"paths"`
	HostConfig  *plugin_mgr.HostConfig      `json:"host_config,omitempty"`
	Migrations  *plugin_mgr.MigrationRecord `json:"migrations,omitempty"`
}

type regPluginRecord struct {
	Current  string                      `json:"current"`
	Versions map[string]regVersionRecord `json:"versions"`
}

type regSnapshot struct {
	Plugins map[string]regPluginRecord `json:"plugins"`
}

type JSONRegistry struct {
	mu   sync.RWMutex
	path string
	mem  regSnapshot
}

func NewJSONRegistry(path string) *JSONRegistry {
	return &JSONRegistry{
		path: path,
		mem:  regSnapshot{Plugins: map[string]regPluginRecord{}},
	}
}

func (r *JSONRegistry) Load(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			r.mem = regSnapshot{Plugins: map[string]regPluginRecord{}}
			return nil
		}
		return plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("registry_load"), plugin_mgr.WithPath(r.path))
	}
	var snap regSnapshot
	if len(bytes.TrimSpace(data)) == 0 {
		snap = regSnapshot{Plugins: map[string]regPluginRecord{}}
	} else if err := json.Unmarshal(data, &snap); err != nil {
		if legacySnap, convErr := decodeLegacySnapshot(data, r.path); convErr != nil {
			return plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("registry_load"), plugin_mgr.WithPath(r.path))
		} else {
			snap = legacySnap
		}
	}
	if snap.Plugins == nil {
		snap.Plugins = map[string]regPluginRecord{}
	}
	r.mem = snap
	return nil
}

func (r *JSONRegistry) Save(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dir := filepath.Dir(r.path)
	_ = os.MkdirAll(dir, 0o755)

	data, err := json.MarshalIndent(r.mem, "", "  ")
	if err != nil {
		return plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("registry_save"))
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("registry_save"), plugin_mgr.WithPath(tmp))
	}
	if err := os.Rename(tmp, r.path); err != nil {
		return plugin_mgr.Wrap(plugin_mgr.CodeIOError, err, plugin_mgr.WithOp("registry_save"), plugin_mgr.WithPath(r.path))
	}
	return nil
}

func (r *JSONRegistry) Put(ctx context.Context, desc Descriptor, state plugin_mgr.PluginState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id, ver := desc.Manifest.ID, desc.Manifest.Version
	if id == "" || ver == "" {
		return plugin_mgr.NewError(plugin_mgr.CodeRegistryError, plugin_mgr.WithOp("registry_put"), plugin_mgr.WithMsg("empty id/version"))
	}

	rec, ok := r.mem.Plugins[id]
	if !ok {
		rec = regPluginRecord{Versions: map[string]regVersionRecord{}}
	}
	rec.Versions[ver] = regVersionRecord{
		State:       state,
		InstalledAt: time.Now().UTC(),
		Manifest:    desc.Manifest,
		Paths:       desc.Paths,
		HostConfig:  cloneHostConfig(desc.HostConfig),
		Migrations:  cloneMigrationRecord(desc.Migration),
	}
	// 注意：installed 不更新 current；enable 时再更新
	r.mem.Plugins[id] = rec
	return nil
}

func (r *JSONRegistry) UpdateState(ctx context.Context, id, version string, state plugin_mgr.PluginState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.mem.Plugins[id]
	if !ok {
		return plugin_mgr.NewError(plugin_mgr.CodeNotFound, plugin_mgr.WithOp("registry_update_state"), plugin_mgr.WithPlugin(id))
	}
	verRec, ok := rec.Versions[version]
	if !ok {
		return plugin_mgr.NewError(plugin_mgr.CodeNotFound, plugin_mgr.WithOp("registry_update_state"), plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(version))
	}
	verRec.State = state
	rec.Versions[version] = verRec

	if state == plugin_mgr.StateEnabled {
		rec.Current = version
	} else if state == plugin_mgr.StateDisabled && rec.Current == version {
		rec.Current = "" // MVP：直接清空
	}
	r.mem.Plugins[id] = rec
	return nil
}

func (r *JSONRegistry) Remove(ctx context.Context, id, version string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.mem.Plugins[id]
	if !ok {
		return nil
	}
	delete(rec.Versions, version)
	if rec.Current == version {
		rec.Current = ""
	}
	if len(rec.Versions) == 0 {
		delete(r.mem.Plugins, id)
	} else {
		r.mem.Plugins[id] = rec
	}
	return nil
}

func (r *JSONRegistry) Get(ctx context.Context, id string) (plugin_mgr.Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, ok := r.mem.Plugins[id]
	if !ok || len(rec.Versions) == 0 {
		return plugin_mgr.Plugin{}, false
	}
	// 选一个版本返回：优先 current；否则挑安装时间最新的
	verKey := rec.Current
	if verKey == "" {
		var latest string
		var latestAt time.Time
		for v, rr := range rec.Versions {
			if rr.InstalledAt.After(latestAt) {
				latestAt = rr.InstalledAt
				latest = v
			}
		}
		verKey = latest
	}
	rr := rec.Versions[verKey]
	return plugin_mgr.Plugin{
		ID:          rr.Manifest.ID,
		Version:     rr.Manifest.Version,
		State:       rr.State,
		Runtime:     rr.Manifest.Runtime,
		Frontend:    rr.Manifest.Frontend,
		Endpoints:   rr.Manifest.Endpoints,
		RBAC:        rr.Manifest.RBAC,
		Events:      rr.Manifest.Events,
		Backend:     rr.Manifest.Backend,
		Routes:      rr.Manifest.Routes,
		Permissions: append([]plugin_mgr.PermissionSpec(nil), rr.Manifest.Permissions...),
		Agents:      append([]plugin_mgr.AgentSpec(nil), rr.Manifest.Agents...),
		Tools:       append([]plugin_mgr.ToolSpec(nil), rr.Manifest.Tools...),
		Workflows:   append([]plugin_mgr.WorkflowSpec(nil), rr.Manifest.Workflows...),
		Paths:       rr.Paths,
		HostConfig:  cloneHostConfig(rr.HostConfig),
		Migration:   cloneMigrationRecord(rr.Migrations),

		Name:        rr.Manifest.Name,
		Description: rr.Manifest.Description,
		Metadata:    rr.Manifest.Metadata,
	}, true
}

func (r *JSONRegistry) List(ctx context.Context) []plugin_mgr.Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]plugin_mgr.Plugin, 0, len(r.mem.Plugins))
	for id := range r.mem.Plugins {
		if p, ok := r.Get(ctx, id); ok {
			out = append(out, p)
		}
	}
	return out
}

// GetVersion: 读取指定 id@version 的完整信息（含 Paths）
func (r *JSONRegistry) GetVersion(ctx context.Context, id, version string) (plugin_mgr.Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, ok := r.mem.Plugins[id]
	if !ok {
		return plugin_mgr.Plugin{}, false
	}
	vr, ok := rec.Versions[version]
	if !ok {
		return plugin_mgr.Plugin{}, false
	}
	return plugin_mgr.Plugin{
		ID:          vr.Manifest.ID,
		Version:     vr.Manifest.Version,
		State:       vr.State,
		Runtime:     vr.Manifest.Runtime,
		Frontend:    vr.Manifest.Frontend,
		Endpoints:   vr.Manifest.Endpoints,
		RBAC:        vr.Manifest.RBAC,
		Events:      vr.Manifest.Events,
		Backend:     vr.Manifest.Backend,
		Routes:      vr.Manifest.Routes,
		Permissions: append([]plugin_mgr.PermissionSpec(nil), vr.Manifest.Permissions...),
		Agents:      append([]plugin_mgr.AgentSpec(nil), vr.Manifest.Agents...),
		Tools:       append([]plugin_mgr.ToolSpec(nil), vr.Manifest.Tools...),
		Workflows:   append([]plugin_mgr.WorkflowSpec(nil), vr.Manifest.Workflows...),
		Paths:       vr.Paths,
		HostConfig:  cloneHostConfig(vr.HostConfig),
		Migration:   cloneMigrationRecord(vr.Migrations),
		Name:        vr.Manifest.Name,
		Description: vr.Manifest.Description,
		Metadata:    vr.Manifest.Metadata,
	}, true
}

func cloneHostConfig(hc *plugin_mgr.HostConfig) *plugin_mgr.HostConfig {
	if hc == nil {
		return nil
	}
	out := &plugin_mgr.HostConfig{
		ValuesFile:  hc.ValuesFile,
		GeneratedAt: hc.GeneratedAt,
	}
	if len(hc.Values) > 0 {
		out.Values = make(map[string]string, len(hc.Values))
		for k, v := range hc.Values {
			out.Values[k] = v
		}
	} else {
		out.Values = map[string]string{}
	}
	if len(hc.Spec) > 0 {
		out.Spec = cloneAnyMap(hc.Spec)
	}
	return out
}

func cloneMigrationRecord(src *plugin_mgr.MigrationRecord) *plugin_mgr.MigrationRecord {
	if src == nil {
		return nil
	}
	out := &plugin_mgr.MigrationRecord{
		Entry:      src.Entry,
		WorkDir:    src.WorkDir,
		Once:       src.Once,
		Timeout:    src.Timeout,
		Hash:       src.Hash,
		LastStatus: src.LastStatus,
		LastError:  src.LastError,
	}
	if len(src.Args) > 0 {
		out.Args = append([]string(nil), src.Args...)
	}
	if src.ExecutedAt != nil {
		t := *src.ExecutedAt
		out.ExecutedAt = &t
	}
	return out
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = cloneAnyValue(v)
	}
	return dst
}

func cloneAnyValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return cloneAnyMap(val)
	case []any:
		out := make([]any, len(val))
		for i := range val {
			out[i] = cloneAnyValue(val[i])
		}
		return out
	default:
		return val
	}
}

func (r *JSONRegistry) CurrentVersion(ctx context.Context, id string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.mem.Plugins[id]
	if !ok || rec.Current == "" {
		return "", false
	}
	return rec.Current, true
}

// 判断 id@version 是否存在
func (r *JSONRegistry) HasVersion(ctx context.Context, id, ver string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, ok := r.mem.Plugins[id]
	if !ok {
		return false
	}
	_, ok = rec.Versions[ver]
	return ok
}

// 设置 current=version（仅内存；调用方自行 Save）
func (r *JSONRegistry) SetCurrent(ctx context.Context, id, ver string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.mem.Plugins[id]
	if !ok {
		return plugin_mgr.NewError(
			plugin_mgr.CodeNotFound,
			plugin_mgr.WithOp("set_current"),
			plugin_mgr.WithPlugin(id),
		)
	}
	if _, ok := rec.Versions[ver]; !ok {
		return plugin_mgr.NewError(
			plugin_mgr.CodeNotFound,
			plugin_mgr.WithOp("set_current"),
			plugin_mgr.WithPlugin(id),
			plugin_mgr.WithVersion(ver),
		)
	}
	// 注意：regPluginRecord 是值类型，修改后要写回 map
	rec.Current = ver
	r.mem.Plugins[id] = rec
	return nil
}

func decodeLegacySnapshot(raw []byte, registryPath string) (regSnapshot, error) {
	type legacyPlugin struct {
		ID          string                 `json:"id"`
		Version     string                 `json:"version"`
		State       string                 `json:"state"`
		InstalledAt string                 `json:"installed_at"`
		Root        string                 `json:"root"`
		Manifest    plugin_mgr.Manifest    `json:"manifest"`
		Extra       map[string]interface{} `json:"-"`
	}
	type legacySnapshot struct {
		Plugins []legacyPlugin `json:"plugins"`
	}

	var legacy legacySnapshot
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return regSnapshot{}, err
	}

	snap := regSnapshot{Plugins: map[string]regPluginRecord{}}
	baseDir := filepath.Dir(registryPath)

	for _, lp := range legacy.Plugins {
		id := strings.TrimSpace(lp.ID)
		ver := strings.TrimSpace(lp.Version)
		if id == "" || ver == "" {
			continue
		}

		state := plugin_mgr.StateInstalled
		switch strings.ToLower(strings.TrimSpace(lp.State)) {
		case string(plugin_mgr.StateEnabled):
			state = plugin_mgr.StateEnabled
		case string(plugin_mgr.StateDisabled):
			state = plugin_mgr.StateDisabled
		case string(plugin_mgr.StateInstalled), "":
			state = plugin_mgr.StateInstalled
		default:
			state = plugin_mgr.StateInstalled
		}

		var installedAt time.Time
		if ts := strings.TrimSpace(lp.InstalledAt); ts != "" {
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				installedAt = t
			}
		}

		root := strings.TrimSpace(lp.Root)
		if root != "" {
			if filepath.IsAbs(root) {
				root = filepath.Clean(root)
			} else {
				root = filepath.Clean(filepath.Join(baseDir, root))
			}
		}

		manifest := lp.Manifest
		if manifest.ID == "" {
			manifest.ID = id
		}
		if manifest.Version == "" {
			manifest.Version = ver
		}

		rec, ok := snap.Plugins[id]
		if !ok {
			rec = regPluginRecord{Versions: map[string]regVersionRecord{}}
		}
		rec.Versions[ver] = regVersionRecord{
			State:       state,
			InstalledAt: installedAt,
			Manifest:    manifest,
			Paths: plugin_mgr.InstalledPaths{
				Root: root,
			},
		}
		if state == plugin_mgr.StateEnabled {
			rec.Current = ver
		}
		snap.Plugins[id] = rec
	}

	return snap, nil
}
