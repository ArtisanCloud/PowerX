// Package authorizationbaseline 管理与业务订单无关的连续授权事实。
package authorizationbaseline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/google/uuid"
)

const (
	Allowed           = "allowed"
	Denied            = "denied"
	UnsupportedScope  = "IAM_BASELINE_UNSUPPORTED_SCOPE"
	UnsupportedPolicy = "IAM_BASELINE_UNSUPPORTED_POLICY"
	RootBypass        = "IAM_BASELINE_ROOT_BYPASS_UNSUPPORTED"
	Unavailable       = "IAM_BASELINE_UNAVAILABLE"
)

// Source 是一条完整授权路径，有效期已经对角色绑定与成员关系求交。
type Source struct {
	ID          string
	From        time.Time
	Until       *time.Time
	Scope       im.DataScope
	Conditional bool
	Effect      string
}

func Digest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func Micro(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }
func ScopeFingerprint(tenant string) string {
	return "sha256:" + Digest([]string{"powerx.iam.scope/v1", tenant, "TENANT"})
}
func Revision(s im.AuthorizationStream, segment im.AuthorizationSegment) string {
	return Digest([]any{"powerx.iam.authorization-baseline/v1", s.UUID, s.FactRevision, segment.UUID, segment.EpochUUID, s.ScopeFingerprint})
}

func decision(sources []Source, at time.Time, root bool) (string, string) {
	active := []Source{}
	for _, s := range sources {
		if !at.Before(s.From) && (s.Until == nil || at.Before(*s.Until)) {
			active = append(active, s)
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i].ID < active[j].ID })
	hash := Digest(active)
	allow, restricted, conditional := false, false, false
	for _, s := range active {
		if s.Effect == "deny" && !s.Conditional && s.Scope == im.ScopeTenant {
			return Denied, hash
		}
		if s.Conditional || (s.Effect != "allow" && s.Effect != "deny") {
			conditional = true
			continue
		}
		if s.Scope != im.ScopeTenant {
			restricted = true
			continue
		}
		if s.Effect == "deny" {
			return Denied, hash
		}
		allow = true
	}
	if conditional {
		return UnsupportedPolicy, hash
	}
	// 受限 deny 可能从 TENANT allow 中减去数据，不能当成完整租户授权。
	for _, s := range active {
		if s.Effect == "deny" && s.Scope != im.ScopeTenant {
			return UnsupportedScope, hash
		}
	}
	if allow {
		return Allowed, hash
	}
	if restricted {
		return UnsupportedScope, hash
	}
	if root {
		return RootBypass, hash
	}
	return Denied, hash
}

// Plan 对所有已保存边界一次求值；时钟前进不需要写入数据库。
func Plan(sources []Source, from time.Time, root bool) []im.AuthorizationSegment {
	from = Micro(from)
	bounds := []time.Time{from}
	for _, s := range sources {
		if s.From.After(from) {
			bounds = append(bounds, Micro(s.From))
		}
		if s.Until != nil && s.Until.After(from) {
			bounds = append(bounds, Micro(*s.Until))
		}
	}
	sort.Slice(bounds, func(i, j int) bool { return bounds[i].Before(bounds[j]) })
	unique := bounds[:0]
	for _, b := range bounds {
		if len(unique) == 0 || !b.Equal(unique[len(unique)-1]) {
			unique = append(unique, b)
		}
	}
	out := []im.AuthorizationSegment{}
	for i, b := range unique {
		state, hash := decision(sources, b, root)
		var until *time.Time
		if i+1 < len(unique) {
			v := unique[i+1]
			until = &v
		}
		if len(out) > 0 && out[len(out)-1].State == state && out[len(out)-1].SourcesHash == hash {
			out[len(out)-1].EffectiveUntil = until
			continue
		}
		out = append(out, im.AuthorizationSegment{EffectiveFrom: b, EffectiveUntil: until, State: state, SourcesHash: hash})
	}
	return out
}

// AssignEpochs 为连续允许分段分配同一个 epoch，并保留当前仍连续的身份。
func AssignEpochs(plan []im.AuthorizationSegment, previous *im.AuthorizationSegment, initialized bool) {
	uncertain := previous != nil && (previous.State == UnsupportedScope || previous.State == UnsupportedPolicy || previous.State == Unavailable)
	for i := range plan {
		p := &plan[i]
		p.UUID = uuid.New()
		switch p.State {
		case UnsupportedScope, UnsupportedPolicy, Unavailable:
			uncertain = true
		case Denied, RootBypass:
			// Root 全局旁路不构成个人成员授权来源，成员权限已明确中断。
			uncertain = false
		}
		if p.State != Allowed {
			continue
		}
		if uncertain {
			// 当前能算出 allow，不代表能够证明此前未知区间是否连续。
			p.State = Unavailable
			continue
		}
		var prior *im.AuthorizationSegment
		if i > 0 && plan[i-1].State == Allowed {
			prior = &plan[i-1]
		} else if i == 0 && previous != nil && previous.State == Allowed {
			prior = previous
		}
		if prior != nil {
			p.EpochUUID = prior.EpochUUID
			p.BaselineKind = prior.BaselineKind
			p.GrantedAt = prior.GrantedAt
			p.VisibleSince = prior.VisibleSince
		} else {
			p.EpochUUID = uuid.NewString()
			v := p.EffectiveFrom
			p.VisibleSince = &v
			p.BaselineKind = "native"
			p.GrantedAt = &v
			if initialized && i == 0 {
				p.BaselineKind = "initialized"
				p.GrantedAt = nil
			}
		}
	}
	for i := 0; i < len(plan); {
		if plan[i].State != Allowed {
			i++
			continue
		}
		j := i + 1
		for j < len(plan) && plan[j].State == Allowed {
			j++
		}
		for k := i; k < j; k++ {
			if j < len(plan) && (plan[j].State == Denied || plan[j].State == RootBypass) {
				plan[k].ValidUntil = plan[j-1].EffectiveUntil
			}
		}
		i = j
	}
}
