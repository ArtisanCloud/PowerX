package storagelifecycle

import (
	"context"
	"sort"
	"sync"
)

// Publication 是数据库批准的只读执行快照，不是普通 JSON 配置入口。
type Publication struct {
	Version      int    `json:"version"`
	HostID       string `json:"host_id"`
	FilesystemID string `json:"filesystem_id"`
	InstanceID   string `json:"instance_id"`
	Digest       string `json:"digest"`
	Policy       Policy `json:"policy"`
}
type Authority struct {
	Resolve func(context.Context, Source, Policy, *Publication) (Policy, error)
	Fence   func(context.Context, Source, Policy, func() error) error
}

// authorityState 仅在 bootstrap 安装一次；执行期间不允许替换策略权威。
type authorityState struct {
	sync.RWMutex
	value *Authority
}

func (e *Engine) BindAuthority(a Authority) error {
	if a.Resolve == nil || a.Fence == nil {
		return ErrInvalid
	}
	e.authority.Lock()
	defer e.authority.Unlock()
	if e.authority.value != nil {
		return ErrConflict
	}
	e.authority.value = &a
	return nil
}
func (e *Engine) policyAuthority() *Authority {
	e.authority.RLock()
	defer e.authority.RUnlock()
	return e.authority.value
}
func (e *Engine) RegisteredSources() []Source {
	out := make([]Source, 0, len(e.sources))
	for _, s := range e.sources {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func (e *Engine) Source(id string) (Source, bool) { s, ok := e.sources[id]; return s, ok }
func (e *Engine) DefaultPolicy(id string) Policy {
	s := e.sources[id]
	mode := "expire_delete"
	if s.RequireArchive {
		mode = "archive_then_delete"
	}
	return Policy{SourceID: id, RetentionDays: 30, MaxBytes: 1 << 30, CleanupMode: mode, RequireArchive: s.RequireArchive, OwnerInstance: e.opts.InstanceID, Fingerprint: s.Fingerprint}
}
func (e *Engine) rawPolicy(d *document, id string) Policy {
	if e.policyAuthority() != nil {
		if p, ok := d.Published[id]; ok {
			return p.Policy
		}
	}
	return d.Policies[id]
}
func (e *Engine) resolvePolicy(ctx context.Context, d *document, id string) (Policy, error) {
	if a := e.policyAuthority(); a != nil && e.sources[id].Supported {
		legacy, ok := d.Policies[id]
		if !ok {
			legacy = e.DefaultPolicy(id)
		}
		var published *Publication
		if p, ok := d.Published[id]; ok {
			published = &p
		}
		return a.Resolve(ctx, e.sources[id], legacy, published)
	}
	return d.Policies[id], nil
}
func (e *Engine) PolicySnapshot(ctx context.Context, id string) (Policy, *Publication, error) {
	if _, ok := e.sources[id]; !ok {
		return Policy{}, nil, ErrInvalid
	}
	d, err := e.load(ctx)
	if err != nil {
		return Policy{}, nil, err
	}
	legacy, ok := d.Policies[id]
	if !ok {
		legacy = e.DefaultPolicy(id)
	}
	if p, ok := d.Published[id]; ok {
		return legacy, &p, nil
	}
	return legacy, nil, nil
}

// ValidatePolicy 复用原生归档、来源、旧执行器及未确认运行保护；不保存或执行。
func (e *Engine) ValidatePolicy(ctx context.Context, actor uint64, id string, revision int64, p Policy) (Policy, error) {
	p, err := e.preparePolicy(ctx, actor, id, p)
	if err != nil {
		return p, err
	}
	d, err := e.load(ctx)
	if err != nil {
		return p, err
	}
	for _, r := range d.Runs {
		if p.Enabled && r.SourceID == id && (r.State == "executing" || r.State == "unconfirmed" && r.AcknowledgedAt == nil) {
			return p, ErrBlocked
		}
	}
	p.Revision = revision
	return p, nil
}
func (e *Engine) PublishPolicy(ctx context.Context, id string, p Publication) error {
	s, ok := e.sources[id]
	if !ok || !s.Supported || p.Version != 1 || p.InstanceID != e.opts.InstanceID || p.Policy.SourceID != id || p.Policy.OwnerInstance != e.opts.InstanceID || p.Policy.Fingerprint != s.Fingerprint || p.Policy.Revision < 1 || p.Digest == "" || p.Policy.ConfigurationDigest != p.Digest {
		return ErrInvalid
	}
	return e.update(ctx, func(d *document) error {
		if old, ok := d.Published[id]; ok {
			if old.HostID != p.HostID || old.FilesystemID != p.FilesystemID || old.InstanceID != p.InstanceID || old.Policy.OwnerInstance != p.Policy.OwnerInstance || old.Policy.Fingerprint != p.Policy.Fingerprint {
				return ErrBlocked
			}
			if old.Policy.Revision > p.Policy.Revision || old.Policy.Revision == p.Policy.Revision && old.Digest != p.Digest {
				return ErrConflict
			}
		}
		d.Published[id] = p
		return nil
	})
}
