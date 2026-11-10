package archive_ops

import (
	"context"
	"sort"
	"time"

	recovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	"github.com/google/uuid"
)

// CommandProbeBinding 仅存原探测目标的配置和指纹，绝不存凭据值。
type CommandProbeBinding struct {
	TargetID      string         `json:"target_id"`
	Revision      int64          `json:"revision"`
	ControlDigest string         `json:"control_digest,omitempty"`
	Settings      TargetSettings `json:"settings"`
	Fingerprint   string         `json:"fingerprint"`
}

// ProbeControlSnapshot 来自完整发布的策略/目标快照；Digest绑定整份配置，防止新增引用溜过核对。
type ProbeControlSnapshot struct {
	Authority      string
	Digest         string
	PolicyRevision int64
	TargetRevision int64
	Target         *Target
	Policies       []Policy
}

// ProbeCommandControl 扩展已有配置权威，以同一事务按策略→目标固定顺序核验。
type ProbeCommandControl interface {
	ObserveProbeCommand(context.Context, string) (ProbeControlSnapshot, error)
	GuardProbeCommand(context.Context, string, ProbeControlSnapshot, func(ProbeControlSnapshot) error) error
}
type ProbePolicyReview struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Revision int64  `json:"revision"`
	Paused   bool   `json:"paused"`
}

// ProbeScopeReview 展示原目标与当前范围，不授予当前目标验证或远端成功资格。
type ProbeScopeReview struct {
	TargetID         string               `json:"target_id"`
	Name             string               `json:"name"`
	OriginalRevision int64                `json:"original_revision"`
	CurrentRevision  int64                `json:"current_revision"`
	TargetState      string               `json:"target_state"`
	Policies         []ProbePolicyReview  `json:"policies"`
	Digest           string               `json:"digest"`
	Snapshot         ProbeControlSnapshot `json:"-"`
	Fingerprint      string               `json:"-"`
}

func validProbeBinding(p *CommandProbeBinding, id string) bool {
	return p != nil && validID(p.TargetID) && id == commandProbeID(p.TargetID) && p.Revision >= 0 && (p.ControlDigest == "" || shaPattern.MatchString(p.ControlDigest)) && shaPattern.MatchString(p.Fingerprint) && (p.Settings.Driver == "ssh" || p.Settings.Driver == "") && ValidateTargetSettings(p.Settings) == nil
}
func (s *Service) initializeProbeCommand(t Target) (string, error) {
	if !validID(t.ID) || t.Driver != "" && t.Driver != "ssh" {
		return "", failure(400, "ARCHIVE_PROBE_SCOPE_INVALID", "仅为固定SSH目标探测建立命令范围")
	}
	fingerprint, e := s.targetFingerprint(t)
	if e != nil {
		return "", e
	}
	id := commandProbeID(t.ID)
	binding := &CommandProbeBinding{t.ID, t.Revision, t.ControlDigest, SettingsOfTarget(t), fingerprint}
	if !validProbeBinding(binding, id) {
		return "", failure(400, "ARCHIVE_PROBE_SCOPE_INVALID", "探测目标绑定无效")
	}
	return id, s.initializeCommandRecord(id, uuid.NewString(), false, binding)
}
func (s *Service) observeProbeControl(ctx context.Context, targetID string) (ProbeControlSnapshot, error) {
	if s.policyControlRequired() || s.targetControlRequired() {
		control, ok := s.policyAuthority.(ProbeCommandControl)
		if !ok {
			return ProbeControlSnapshot{}, failure(503, "ARCHIVE_PROBE_CONTROL_UNAVAILABLE", "探测范围的正式配置权威尚未就绪")
		}
		return control.ObserveProbeCommand(ctx, targetID)
	}
	targets, e := s.LegacyTargets(ctx)
	if e != nil {
		return ProbeControlSnapshot{}, e
	}
	ids, e := s.policyIDs(ctx)
	if e != nil {
		return ProbeControlSnapshot{}, e
	}
	if len(ids) > 128 {
		return ProbeControlSnapshot{}, failure(503, "ARCHIVE_POLICY_CATALOG_LIMIT", "策略目录超限")
	}
	sort.Strings(ids)
	all := []Policy{}
	refs := []Policy{}
	for _, id := range ids {
		p, e := s.policy(ctx, id)
		if e != nil {
			return ProbeControlSnapshot{}, e
		}
		all = append(all, p)
		if p.TargetID == targetID {
			refs = append(refs, p)
		}
	}
	out := ProbeControlSnapshot{Authority: "local", Policies: refs, Digest: dispositionHash([]any{targets, all})}
	for _, t := range targets {
		if t.ID == targetID {
			copy := t
			out.Target = &copy
			break
		}
	}
	return out, nil
}
func (s *Service) probeScopeReview(r commandRecord, snapshot ProbeControlSnapshot) (*ProbeScopeReview, error) {
	if !validProbeBinding(r.Probe, r.ID) {
		return nil, failure(409, "ARCHIVE_PROBE_SCOPE_MISSING", "缺少原目标绑定，不能挪用任务或补造范围")
	}
	out := &ProbeScopeReview{TargetID: r.Probe.TargetID, Name: r.Probe.Settings.Name, OriginalRevision: r.Probe.Revision, TargetState: "deleted", Policies: []ProbePolicyReview{}, Digest: snapshot.Digest, Snapshot: snapshot}
	if snapshot.Target != nil {
		t := *snapshot.Target
		out.CurrentRevision = t.Revision
		out.TargetState = "changed"
		if t.Driver == "" || t.Driver == "ssh" {
			fingerprint, e := s.targetFingerprint(t)
			if e != nil {
				return nil, failure(503, "ARCHIVE_PROBE_CREDENTIAL_UNAVAILABLE", "当前目标凭据指纹不可核对，继续保护")
			}
			out.Fingerprint = fingerprint
			if t.Revision == r.Probe.Revision && t.ControlDigest == r.Probe.ControlDigest && SettingsOfTarget(t) == r.Probe.Settings && fingerprint == r.Probe.Fingerprint {
				out.TargetState = "unchanged"
			}
		}
	}
	for _, p := range snapshot.Policies {
		out.Policies = append(out.Policies, ProbePolicyReview{p.ID, p.Name, p.Revision, !p.Enabled && !p.RemoteCleanupEnabled})
	}
	out.Digest = dispositionHash([]any{snapshot.Digest, out.Fingerprint})
	sort.Slice(out.Policies, func(i, j int) bool { return out.Policies[i].ID < out.Policies[j].ID })
	return out, nil
}
func (s *Service) probeCommandReviewLocked(ctx context.Context, r commandRecord) (CommandDrainReview, commandRecord, jobRecord, Policy, recovery.Epoch, error) {
	var out CommandDrainReview
	epoch, e := s.dispositionEpoch(ctx)
	if e != nil {
		return out, r, jobRecord{}, Policy{}, epoch, e
	}
	facts := observeRecordedCommand(r)
	out = CommandDrainReview{RecordID: r.ID, JobID: "", ExecutionID: r.ExecutionID, CommandID: r.CommandID, Kind: r.Kind, State: r.State, Eligible: false, Problem: "probe_scope_missing", BootMatches: facts.BootMatches, Leader: facts.Leader, Group: facts.Group, Identity: r.Identity, ReadOnly: true, ObservedAt: time.Now().UTC(), Subject: "probe"}
	if r.Protocol == 3 && validProbeBinding(r.Probe, r.ID) {
		snapshot, e := s.observeProbeControl(ctx, r.Probe.TargetID)
		if e != nil {
			return out, r, jobRecord{}, Policy{}, epoch, e
		}
		view, e := s.probeScopeReview(r, snapshot)
		if e != nil {
			return out, r, jobRecord{}, Policy{}, epoch, e
		}
		out.Probe = view
		out.TargetID = r.Probe.TargetID
		out.Problem = commandReviewProblem(s.hostIdentity, r, facts)
		for _, p := range view.Policies {
			if !p.Paused {
				out.Problem = "probe_policies_active"
			}
		}
		out.Eligible = out.Problem == ""
	}
	out.ReviewVersion = dispositionHash([]any{"probe-command-confirmation/v1", s.cfg.Instance, s.hostIdentity, r, epoch, facts, out.Probe})
	return out, r, jobRecord{}, Policy{}, epoch, nil
}
func (s *Service) guardCommandReview(ctx context.Context, r commandRecord, p Policy, view *ProbeScopeReview, action func(context.Context) error) error {
	if r.JobID != "" {
		return s.guardPolicyAction(ctx, p, "interruption-retain", action)
	}
	if view == nil {
		return action(ctx)
	} // 旧记录仅能查看缺证说明，Eligible 始终为 false。
	check := func(snapshot ProbeControlSnapshot) error {
		current, e := s.probeScopeReview(r, snapshot)
		if e != nil {
			return e
		}
		if dispositionHash(current) != dispositionHash(view) {
			return failure(409, "ARCHIVE_REVIEW_STALE", "目标、凭据或引用策略发生变化，请重新核对")
		}
		// 在权威快照锁内取得完整引用集合的执行锁，不能沿用预览前的旧集合。
		releases := []func(){}
		defer func() {
			for i := len(releases) - 1; i >= 0; i-- {
				releases[i]()
			}
		}()
		for _, policy := range current.Policies {
			if !policy.Paused {
				return failure(409, "ARCHIVE_PAUSE_REQUIRED", "所有引用原目标的策略必须明确暂停")
			}
			release, e := s.lock(ctx, policy.ID)
			if e != nil {
				return failure(409, "ARCHIVE_BUSY", "引用原目标的策略执行尚未排空")
			}
			releases = append(releases, release)
		}
		return action(ctx)
	}
	if s.policyControlRequired() || s.targetControlRequired() {
		control, ok := s.policyAuthority.(ProbeCommandControl)
		if !ok {
			return failure(503, "ARCHIVE_PROBE_CONTROL_UNAVAILABLE", "探测配置权威未就绪")
		}
		return control.GuardProbeCommand(ctx, r.Probe.TargetID, view.Snapshot, check)
	}
	snapshot, e := s.observeProbeControl(ctx, r.Probe.TargetID)
	if e != nil {
		return e
	}
	return check(snapshot)
}
func commandReviewProblem(host string, r commandRecord, f commandOSObservation) string {
	switch {
	case r.Host != host:
		return "host_changed"
	case r.State != "running":
		return "command_not_unconfirmed"
	case (r.Protocol != 2 && r.Protocol != 3) || !validCommandIdentity(r.Identity):
		return "identity_missing"
	case !f.BootMatches:
		return "boot_changed"
	case f.Leader == "identity_mismatch":
		return "identity_mismatch"
	case f.Leader != "absent" || f.Group != "absent":
		return "group_not_drained"
	}
	return ""
}
