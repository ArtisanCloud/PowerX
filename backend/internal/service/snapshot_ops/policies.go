package snapshot_ops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const legacyPolicyID = "be7d75ac-57ec-4c90-a513-000000000001"

type policyRecord struct {
	Policy
	ID     string    `json:"id,omitempty"`
	Name   string    `json:"name,omitempty"`
	Anchor time.Time `json:"schedule_anchor,omitempty"`
}

// 所有策略及默认选择在同一账本中原子提交，跨进程同样不允许两个启用项。
type policyState struct {
	Version   int            `json:"version"`
	DefaultID string         `json:"default_policy_id"`
	Policies  []policyRecord `json:"policies"`
}

func defaultPolicy() Policy {
	return Policy{IntervalMinutes: 1440, RetentionDays: 7, RetentionCount: 7}
}

func validatePolicy(p Policy) error {
	if p.IntervalMinutes < 1 || p.IntervalMinutes > 525600 || p.RetentionDays < 1 || p.RetentionDays > 3650 || p.RetentionCount < 1 || p.RetentionCount > 10000 || p.RetentionDays*1440 < p.IntervalMinutes {
		return failure(400, "SNAPSHOT_POLICY_INVALID", "保留窗口不得短于采集周期，且周期和份数必须有效")
	}
	return nil
}

func (s *Service) policyState() (policyState, error) {
	out := policyState{Version: 1, Policies: []policyRecord{}}
	file := filepath.Join(s.cfg.Directory, "policies.json")
	if _, e := os.Lstat(file); e == nil {
		out = policyState{}
		if e = readJSON(file, &out); e != nil {
			return out, e
		}
		if out.Version != 1 || len(out.Policies) > 100 {
			return out, failure(503, "SNAPSHOT_POLICY_STORE_INVALID", "快照策略账本版本或数量无效")
		}
		ids, names := map[string]bool{}, map[string]bool{}
		foundDefault := out.DefaultID == "" && len(out.Policies) == 0
		for _, p := range out.Policies {
			name := strings.ToLower(strings.TrimSpace(p.Name))
			if !validID(p.ID) || name == "" || ids[p.ID] || names[name] || validatePolicy(p.Policy) != nil || (p.Enabled && p.ID != out.DefaultID) {
				return out, failure(503, "SNAPSHOT_POLICY_STORE_INVALID", "快照策略账本存在无效或重复配置")
			}
			ids[p.ID], names[name] = true, true
			foundDefault = foundDefault || p.ID == out.DefaultID
		}
		if !foundDefault {
			return out, failure(503, "SNAPSHOT_POLICY_STORE_INVALID", "默认快照策略不存在")
		}
		return out, nil
	} else if !errors.Is(e, os.ErrNotExist) {
		return out, e
	}
	// 兼容已有单策略，不在读取接口改写文件。首次策略写入时封存到新账本，原文件保留。
	legacy := policyRecord{Policy: defaultPolicy(), ID: legacyPolicyID, Name: "默认快照策略"}
	if e := readJSON(filepath.Join(s.cfg.Directory, "policy.json"), &legacy); e != nil {
		if _, statErr := os.Lstat(filepath.Join(s.cfg.Directory, "policy.json")); errors.Is(statErr, os.ErrNotExist) {
			return out, nil
		}
		return out, e
	}
	if e := validatePolicy(legacy.Policy); e != nil {
		return out, e
	}
	legacy.ID, legacy.Name = legacyPolicyID, "默认快照策略"
	out.DefaultID, out.Policies = legacy.ID, []policyRecord{legacy}
	return out, nil
}

func (state policyState) current() policyRecord {
	for _, p := range state.Policies {
		if p.ID == state.DefaultID {
			return p
		}
	}
	return policyRecord{Policy: defaultPolicy()}
}

func (s *Service) policyRecord() (policyRecord, error) {
	state, e := s.policyState()
	return state.current(), e
}
func (s *Service) policy() (Policy, error) { p, e := s.policyRecord(); return p.Policy, e }

func nextPolicyRun(p policyRecord, snapshots []Snapshot) *time.Time {
	if !p.Enabled {
		return nil
	}
	anchor := p.Anchor
	for _, j := range snapshots {
		if !j.Imported && (j.PolicyID == p.ID || (j.PolicyID == "" && p.ID == legacyPolicyID)) && j.CreatedAt.After(anchor) {
			anchor = j.CreatedAt
		}
	}
	next := anchor.Add(time.Duration(p.IntervalMinutes) * time.Minute)
	return &next
}

func exposePolicy(p policyRecord, defaultID string) NamedPolicy {
	return NamedPolicy{Policy: p.Policy, ID: p.ID, Name: p.Name, Default: p.ID == defaultID}
}

// SavePolicy 兼容旧单策略接口，只更新当前默认策略。
func (s *Service) SavePolicy(ctx context.Context, p Policy) (Policy, error) {
	var saved Policy
	e := s.changePolicies(ctx, "policy", "", func(state *policyState) error {
		if e := s.validatePolicyInput(ctx, p); e != nil {
			return e
		}
		current := state.current()
		if current.ID == "" {
			current.ID, current.Name = legacyPolicyID, "默认快照策略"
			state.DefaultID = current.ID
			state.Policies = append(state.Policies, current)
		}
		for i := range state.Policies {
			if state.Policies[i].ID == current.ID {
				applyPolicy(&state.Policies[i], p, s.opts.Now())
				saved = state.Policies[i].Policy
			}
		}
		return nil
	})
	return saved, e
}

func (s *Service) validatePolicyInput(ctx context.Context, p Policy) error {
	if e := validatePolicy(p); e != nil {
		return e
	}
	if p.Enabled {
		if problems := s.problems(ctx); len(problems) > 0 {
			return failure(422, "SNAPSHOT_SOURCE_UNAVAILABLE", strings.Join(problems, "；"))
		}
	}
	return nil
}

func applyPolicy(record *policyRecord, p Policy, now time.Time) {
	if p.Enabled && (!record.Enabled || record.IntervalMinutes != p.IntervalMinutes || record.Anchor.IsZero()) {
		record.Anchor = now
	}
	record.Policy = p
}

func (s *Service) changePolicies(ctx context.Context, action, id string, change func(*policyState) error) error {
	if e := s.check(ctx); e != nil {
		return e
	}
	unlock, e := lockFile(filepath.Join(s.cfg.Directory, "locks", "policy.lock"))
	if e != nil {
		return e
	}
	defer unlock()
	state, e := s.policyState()
	if e != nil {
		return e
	}
	if e = change(&state); e != nil {
		return e
	}
	if e = s.audit(ctx, action, id, "success"); e != nil {
		return e
	}
	return atomicJSON(filepath.Join(s.cfg.Directory, "policies.json"), state)
}

// PutPolicy 新增或更新指定配置；启用会在同次原子提交中停用其他配置并设为默认。
func (s *Service) PutPolicy(ctx context.Context, id, name string, p Policy) (NamedPolicy, error) {
	var out NamedPolicy
	creating := id == ""
	if creating {
		id = uuid.NewString()
	}
	e := s.changePolicies(ctx, "policy_save", id, func(state *policyState) error {
		if e := s.validatePolicyInput(ctx, p); e != nil {
			return e
		}
		name = strings.TrimSpace(name)
		if name == "" || utf8.RuneCountInString(name) > 100 || strings.ContainsAny(name, "\r\n\x00") {
			return failure(400, "SNAPSHOT_POLICY_NAME_INVALID", "策略名称须为 1 至 100 个字符")
		}
		index := -1
		for i, prior := range state.Policies {
			if prior.ID == id {
				index = i
			}
			if prior.ID != id && strings.EqualFold(prior.Name, name) {
				return failure(409, "SNAPSHOT_POLICY_NAME_EXISTS", "策略名称已存在")
			}
		}
		if creating {
			if len(state.Policies) >= 100 {
				return failure(400, "SNAPSHOT_POLICY_LIMIT", "最多保存 100 套快照策略")
			}
			state.Policies = append(state.Policies, policyRecord{ID: id})
			index = len(state.Policies) - 1
		} else if !validID(id) || index < 0 {
			return failure(404, "SNAPSHOT_POLICY_NOT_FOUND", "快照策略不存在")
		}
		if state.DefaultID == "" || p.Enabled {
			state.DefaultID = id
		}
		if p.Enabled {
			for i := range state.Policies {
				if i != index {
					state.Policies[i].Enabled = false
				}
			}
		}
		record := &state.Policies[index]
		record.Name = name
		applyPolicy(record, p, s.opts.Now())
		out = exposePolicy(*record, state.DefaultID)
		return nil
	})
	return out, e
}

// SetPolicyEnabled 只切换启停状态，不用页面上的旧表单覆盖周期和保留配置。
func (s *Service) SetPolicyEnabled(ctx context.Context, id string, enabled bool) (NamedPolicy, error) {
	var out NamedPolicy
	e := s.changePolicies(ctx, "policy_status", id, func(state *policyState) error {
		for i := range state.Policies {
			if state.Policies[i].ID != id {
				continue
			}
			p := state.Policies[i].Policy
			p.Enabled = enabled
			if e := s.validatePolicyInput(ctx, p); e != nil {
				return e
			}
			if enabled {
				state.DefaultID = id
				for j := range state.Policies {
					if j != i {
						state.Policies[j].Enabled = false
					}
				}
			}
			applyPolicy(&state.Policies[i], p, s.opts.Now())
			out = exposePolicy(state.Policies[i], state.DefaultID)
			return nil
		}
		return failure(404, "SNAPSHOT_POLICY_NOT_FOUND", "快照策略不存在")
	})
	return out, e
}

// SetDefaultPolicy 仅选择手动采集使用的系统策略，不开启定时任务。
// 默认切换时停用旧策略，避免其在后台继续自动采集。
func (s *Service) SetDefaultPolicy(ctx context.Context, id string) (NamedPolicy, error) {
	var out NamedPolicy
	e := s.changePolicies(ctx, "policy_default", id, func(state *policyState) error {
		index := -1
		for i := range state.Policies {
			if state.Policies[i].ID == id {
				index = i
			}
		}
		if index < 0 {
			return failure(404, "SNAPSHOT_POLICY_NOT_FOUND", "快照策略不存在")
		}
		state.DefaultID = id
		for i := range state.Policies {
			state.Policies[i].Enabled = false
		}
		out = exposePolicy(state.Policies[index], id)
		return nil
	})
	return out, e
}
