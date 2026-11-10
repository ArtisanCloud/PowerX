package storagebudget

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"time"
)

var ErrControlRequired = errors.New("storage budget database authority required")
var ErrControlChanged = errors.New("storage budget authority changed")

// ControlProjection 为同盘协调者的私有投影；普通配置权威仍在数据库。
type ControlProjection struct {
	Version       int             `json:"version"`
	HostID        string          `json:"host_id"`
	FilesystemID  string          `json:"filesystem_id"`
	OwnerInstance string          `json:"owner_instance"`
	AuthorityID   string          `json:"authority_id"`
	Revision      int64           `json:"revision"`
	Digest        string          `json:"digest"`
	Document      json.RawMessage `json:"document"`
	ActorID       uint64          `json:"actor_id"`
	UpdatedAt     time.Time       `json:"updated_at"`
}
type budgetControlBinding struct {
	Version       int    `json:"version"`
	HostID        string `json:"host_id"`
	FilesystemID  string `json:"filesystem_id"`
	OwnerInstance string `json:"owner_instance"`
	AuthorityID   string `json:"authority_id"`
}

func bindingOf(p ControlProjection) budgetControlBinding {
	return budgetControlBinding{1, p.HostID, p.FilesystemID, p.OwnerInstance, p.AuthorityID}
}
func (c *Coordinator) readControlBinding() (*budgetControlBinding, error) {
	raw, e := c.readPrivate("budget-control-binding.json", 4096)
	if errors.Is(e, os.ErrNotExist) {
		return nil, nil
	}
	if e != nil {
		return nil, ErrControlRequired
	}
	var b budgetControlBinding
	if json.Unmarshal(raw, &b) != nil || b.Version != 1 || b.HostID != c.cfg.HostID || b.FilesystemID != c.cfg.FilesystemID || b.OwnerInstance == "" || len(b.AuthorityID) != 36 {
		return nil, ErrControlRequired
	}
	return &b, nil
}

type ControlSnapshot struct {
	HostID       string
	FilesystemID string
	InstanceID   string
	Legacy       BudgetPolicy
	Projection   *ControlProjection
}

// BudgetAuthority.Fence 必须先锁定数据库权威，再调用当前主机账本动作，禁止反向取锁。
type BudgetAuthority struct {
	Fence func(context.Context, ControlSnapshot, func(BudgetPolicy) error) error
}

func (c *Coordinator) BindAuthority(a BudgetAuthority) error {
	c.authorityMu.Lock()
	defer c.authorityMu.Unlock()
	if a.Fence == nil || c.authority.Fence != nil {
		return ErrControlRequired
	}
	c.authority = a
	return nil
}
func (c *Coordinator) authorityFence() func(context.Context, ControlSnapshot, func(BudgetPolicy) error) error {
	c.authorityMu.RLock()
	defer c.authorityMu.RUnlock()
	return c.authority.Fence
}

// controlLocked 只在回调尚未进入时短暂等待，已进入的持久化绝不重放。
func (c *Coordinator) controlLocked(ctx context.Context, write bool, fn func(*ledger) error) error {
	bounded, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	for {
		entered := false
		err := c.locked(bounded, write, func(l *ledger) error { entered = true; return fn(l) })
		if entered || !errors.Is(err, ErrBusy) {
			return err
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-bounded.Done():
			timer.Stop()
			return bounded.Err()
		case <-timer.C:
		}
	}
}
func (c *Coordinator) ControlSnapshot(ctx context.Context) (ControlSnapshot, error) {
	out := ControlSnapshot{HostID: c.cfg.HostID, FilesystemID: c.cfg.FilesystemID, InstanceID: c.cfg.InstanceID}
	err := c.controlLocked(ctx, false, func(l *ledger) error {
		out.Legacy = *l.Policy
		binding, e := c.readControlBinding()
		if e != nil {
			return e
		}
		if l.Control == nil && binding != nil || l.Control != nil && (binding == nil || *binding != bindingOf(*l.Control)) {
			return ErrControlRequired
		}
		if l.Control != nil {
			p := *l.Control
			p.Document = bytes.Clone(p.Document)
			out.Projection = &p
		}
		return nil
	})
	return out, err
}
func (c *Coordinator) withAuthorizedBudget(ctx context.Context, write bool, fn func(*ledger) error) error {
	snapshot, err := c.ControlSnapshot(ctx)
	if err != nil {
		return err
	}
	action := func(p BudgetPolicy) error {
		if !validBudget(p) {
			return ErrUnavailable
		}
		return c.controlLocked(ctx, write, func(l *ledger) error {
			binding, e := c.readControlBinding()
			if e != nil {
				return e
			}
			if l.Control == nil && binding != nil || l.Control != nil && (binding == nil || *binding != bindingOf(*l.Control)) {
				return ErrControlRequired
			}
			if !reflect.DeepEqual(l.Control, snapshot.Projection) || *l.Policy != snapshot.Legacy {
				return ErrControlChanged
			}
			legacy := l.Policy
			l.Policy = &p
			defer func() { l.Policy = legacy }()
			return fn(l)
		})
	}
	fence := c.authorityFence()
	if fence == nil {
		if snapshot.Projection != nil {
			return ErrControlRequired
		}
		return action(snapshot.Legacy)
	}
	return fence(ctx, snapshot, action)
}

// PublishControl 只写固定预算投影，保留旧值作为接管证据；不同安装／数据库不得抢占。
func (c *Coordinator) PublishControl(ctx context.Context, p ControlProjection, origin BudgetPolicy) error {
	if c.authorityFence() == nil || p.Version != 1 || p.HostID != c.cfg.HostID || p.FilesystemID != c.cfg.FilesystemID || p.OwnerInstance == "" || len(p.AuthorityID) != 36 || p.Revision < 1 || len(p.Digest) != 64 || len(p.Document) > 1<<20 || !json.Valid(p.Document) {
		return ErrControlRequired
	}
	return c.controlLocked(ctx, true, func(l *ledger) error {
		binding, e := c.readControlBinding()
		if e != nil {
			return e
		}
		want := bindingOf(p)
		if binding != nil && *binding != want {
			return ErrControlChanged
		}
		if l.Control != nil && binding == nil {
			return ErrControlRequired
		}
		if l.Control == nil {
			if *l.Policy != origin {
				return ErrControlChanged
			}
		} else {
			old := l.Control
			if old.AuthorityID != p.AuthorityID || old.OwnerInstance != p.OwnerInstance || old.HostID != p.HostID || old.FilesystemID != p.FilesystemID || old.Revision > p.Revision || old.Revision == p.Revision && !reflect.DeepEqual(*old, p) {
				return ErrControlChanged
			}
		}
		// 先写不可变绑定意图；中断只会阻止准入，相同发布可安全续做。
		if binding == nil {
			raw, e := json.Marshal(want)
			if e != nil {
				return e
			}
			if e = c.writePrivate("budget-control-binding.json", raw); e != nil {
				return e
			}
		}
		l.Control = &p
		return nil
	})
}

// ValidPolicy 供固定业务 DTO 共用相同范围校验。
func ValidPolicy(p BudgetPolicy) bool { return validBudget(p) }
