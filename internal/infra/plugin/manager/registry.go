package manager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	CurrentVersion(ctx context.Context, id string) (string, bool)
}

type regVersionRecord struct {
	State       plugin_mgr.PluginState    `json:"state"`
	InstalledAt time.Time                 `json:"installed_at"`
	Manifest    plugin_mgr.Manifest       `json:"manifest"`
	Paths       plugin_mgr.InstalledPaths `json:"paths"`
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
	if err := json.Unmarshal(data, &snap); err != nil {
		return plugin_mgr.Wrap(plugin_mgr.CodeRegistryError, err, plugin_mgr.WithOp("registry_load"), plugin_mgr.WithPath(r.path))
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
		ID:        rr.Manifest.ID,
		Version:   rr.Manifest.Version,
		State:     rr.State,
		Runtime:   rr.Manifest.Runtime,
		Frontend:  rr.Manifest.Frontend,
		Endpoints: rr.Manifest.Endpoints,
		RBAC:      rr.Manifest.RBAC,
		Events:    rr.Manifest.Events,
		Paths:     rr.Paths,
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

func (r *JSONRegistry) CurrentVersion(ctx context.Context, id string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.mem.Plugins[id]
	if !ok || rec.Current == "" {
		return "", false
	}
	return rec.Current, true
}
