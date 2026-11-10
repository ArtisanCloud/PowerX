package storagegov

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/gorm"
)

type HostBudgetSettings struct {
	MinFreeBytes      int64 `json:"min_free_bytes"`
	CriticalFreeBytes int64 `json:"critical_free_bytes"`
	MaxUsedBytes      int64 `json:"max_used_bytes"`
	MaxConcurrent     int   `json:"max_concurrent"`
}

func budgetSettings(p budget.BudgetPolicy) HostBudgetSettings {
	return HostBudgetSettings{p.MinFreeBytes, p.CriticalFreeBytes, p.MaxUsedBytes, p.MaxConcurrent}
}
func (p HostBudgetSettings) policy(rev int64) budget.BudgetPolicy {
	return budget.BudgetPolicy{Revision: rev, MinFreeBytes: p.MinFreeBytes, CriticalFreeBytes: p.CriticalFreeBytes, MaxUsedBytes: p.MaxUsedBytes, MaxConcurrent: p.MaxConcurrent}
}

type hostBudgetDocument struct {
	Recovery     *RecoveryStamp      `json:"recovery,omitempty"`
	Version      int                 `json:"version"`
	AuthorityID  string              `json:"authority_id"`
	Settings     HostBudgetSettings  `json:"settings"`
	Origin       budget.BudgetPolicy `json:"origin"`
	OriginDigest string              `json:"origin_digest"`
}
type HostBudgetConfiguration struct {
	Authority         string              `json:"authority"`
	State             string              `json:"state"`
	Ready             bool                `json:"ready"`
	CanManage         bool                `json:"can_manage"`
	OwnerInstance     string              `json:"owner_instance"`
	DesiredRevision   int64               `json:"desired_revision"`
	AppliedRevision   int64               `json:"applied_revision"`
	SnapshotRevision  *int64              `json:"snapshot_revision"`
	DesiredPolicy     *HostBudgetSettings `json:"desired_policy"`
	EffectivePolicy   *HostBudgetSettings `json:"effective_policy"`
	PendingProtection bool                `json:"pending_protection"`
	LegacyRevision    int64               `json:"legacy_revision"`
	LegacyDigest      string              `json:"legacy_digest"`
	Problem           string              `json:"problem,omitempty"`
}
type HostBudgetChange struct {
	RequestID        string             `json:"request_id"`
	ExpectedRevision int64              `json:"expected_revision"`
	Settings         HostBudgetSettings `json:"settings"`
	Confirm          bool               `json:"confirm"`
}
type HostBudgetAdoption struct {
	RequestID      string `json:"request_id"`
	LegacyRevision int64  `json:"legacy_revision"`
	LegacyDigest   string `json:"legacy_digest"`
	Confirm        bool   `json:"confirm"`
}
type HostBudgetMutation struct {
	RequestID         string                  `json:"request_id"`
	OperationRevision int64                   `json:"operation_revision"`
	OperationPolicy   HostBudgetSettings      `json:"operation_policy"`
	Configuration     HostBudgetConfiguration `json:"configuration"`
}

func strictHostBudget(raw []byte) (hostBudgetDocument, error) {
	var d hostBudgetDocument
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&d) != nil || decoder.Decode(new(any)) != io.EOF || d.Version != 1 || !validTargetUUID(d.AuthorityID) || len(d.OriginDigest) != 64 || !budget.ValidPolicy(d.Settings.policy(1)) || !budget.ValidPolicy(d.Origin) {
		return d, controlError(400, "STORAGE_BUDGET_DOCUMENT_INVALID")
	}
	digest, _ := hostBudgetOriginDigest(d.Origin)
	if digest != d.OriginDigest {
		return d, controlError(400, "STORAGE_BUDGET_DOCUMENT_INVALID")
	}
	return d, nil
}
func hostBudgetOriginDigest(p budget.BudgetPolicy) (string, error) {
	raw, e := json.Marshal(p)
	if e != nil {
		return "", e
	}
	raw, e = canonicalControl(raw)
	return controlDigest(raw), e
}
func (s *Service) bindBudgetControl() error {
	if s.opts.Budget == nil {
		return nil
	}
	if e := s.opts.Control.Bind(ControlBudget, ControlBinding{Validate: func(raw json.RawMessage) error { _, e := strictHostBudget(raw); return e }, Apply: func(ctx context.Context, p ControlPublication) error {
		d, e := strictHostBudget(p.Document)
		if e != nil {
			return e
		}
		scope := s.opts.Control.opts.Scope
		if p.Scope != scope {
			return ErrStateUnavailable
		}
		return s.opts.Budget.PublishControl(ctx, budget.ControlProjection{Version: 1, HostID: scope.HostID, FilesystemID: scope.FilesystemID, OwnerInstance: scope.InstanceID, AuthorityID: d.AuthorityID, Revision: p.Revision, Digest: p.Digest, Document: p.Document, ActorID: p.ActorID, UpdatedAt: p.UpdatedAt}, d.Origin)
	}}); e != nil {
		return e
	}
	return s.opts.Budget.BindAuthority(budget.BudgetAuthority{Fence: s.fenceHostBudget})
}

// ownerBudgetControl 仅复用当前正式 DB／TaskBus 读取主机已选定安装的权威，不启动第二消费者。
func (s *Service) ownerBudgetControl(snapshot budget.ControlSnapshot) *ControlService {
	c := s.opts.Control
	if c == nil {
		return nil
	}
	scope := c.opts.Scope
	if snapshot.Projection != nil {
		scope.InstanceID = snapshot.Projection.OwnerInstance
	}
	return &ControlService{repo: c.repo, opts: ControlOptions{Scope: scope, Queue: c.opts.Queue, Dispatch: c.opts.Dispatch}}
}
func conservativeBudget(a, b budget.BudgetPolicy) budget.BudgetPolicy {
	a.MinFreeBytes = max(a.MinFreeBytes, b.MinFreeBytes)
	a.CriticalFreeBytes = max(a.CriticalFreeBytes, b.CriticalFreeBytes)
	if b.MaxUsedBytes > 0 && (a.MaxUsedBytes == 0 || b.MaxUsedBytes < a.MaxUsedBytes) {
		a.MaxUsedBytes = b.MaxUsedBytes
	}
	a.MaxConcurrent = min(a.MaxConcurrent, b.MaxConcurrent)
	return a
}
func (s *Service) loadHostBudget(ctx context.Context, snapshot budget.ControlSnapshot) (HostBudgetConfiguration, ControlState, budget.BudgetPolicy, error) {
	out := HostBudgetConfiguration{Authority: "unavailable", State: "unavailable", OwnerInstance: s.opts.Identity.InstanceID}
	state := ControlState{}
	effective := snapshot.Legacy
	fail := func(code string) (HostBudgetConfiguration, ControlState, budget.BudgetPolicy, error) {
		out.Authority = "unavailable"
		out.State = "unconfirmed"
		out.Problem = code
		out.CanManage = false
		out.EffectivePolicy = nil
		return out, state, effective, controlError(503, code)
	}
	c := s.ownerBudgetControl(snapshot)
	if c == nil || snapshot.HostID != c.opts.Scope.HostID || snapshot.FilesystemID != c.opts.Scope.FilesystemID || !c.Ready(ctx) {
		return fail("STORAGE_CONTROL_SCHEMA_NOT_READY")
	}
	out.Ready = true
	out.OwnerInstance = c.opts.Scope.InstanceID
	out.CanManage = out.OwnerInstance == s.opts.Identity.InstanceID && s.opts.ControlWorkerEnabled && !s.opts.Control.ConsumerExited()
	var e error
	state, e = c.read(ctx, ControlBudget)
	if e != nil {
		return fail("STORAGE_CONTROL_UNAVAILABLE")
	}
	out.LegacyRevision = snapshot.Legacy.Revision
	out.LegacyDigest, _ = hostBudgetOriginDigest(snapshot.Legacy)
	if state.State == "unconfigured" {
		if snapshot.Projection != nil {
			return fail("STORAGE_BUDGET_RESTORE_CONFIRMATION_REQUIRED")
		}
		out.Authority = "legacy"
		out.State = "legacy"
		p := budgetSettings(effective)
		out.EffectivePolicy = &p
		return out, state, effective, nil
	}
	d, e := strictHostBudget(state.Document)
	if e != nil {
		return fail("STORAGE_BUDGET_DOCUMENT_INVALID")
	}
	out.Authority = "database"
	out.State = state.State
	out.DesiredRevision = state.DesiredRevision
	out.AppliedRevision = state.AppliedRevision
	out.DesiredPolicy = &d.Settings
	out.Problem = state.Problem
	if snapshot.Legacy != d.Origin {
		return fail("STORAGE_BUDGET_LEGACY_CHANGED")
	}
	if snapshot.Projection == nil {
		if state.AppliedRevision > 0 {
			return fail("STORAGE_BUDGET_SNAPSHOT_UNCONFIRMED")
		}
	} else {
		p := snapshot.Projection
		if p.Version != 1 || p.HostID != state.Scope.HostID || p.FilesystemID != state.Scope.FilesystemID || p.OwnerInstance != state.Scope.InstanceID || p.AuthorityID != d.AuthorityID || p.Revision != state.AppliedRevision || p.Revision < 1 {
			return fail("STORAGE_BUDGET_SNAPSHOT_UNCONFIRMED")
		}
		revision, e := c.repo.Revision(ctx, state.Scope.ID(), string(ControlBudget), p.Revision)
		if e != nil || revision.Digest != p.Digest || revision.ActorID != p.ActorID || !revision.CreatedAt.Equal(p.UpdatedAt) {
			return fail("STORAGE_BUDGET_SNAPSHOT_UNCONFIRMED")
		}
		if _, e = verifiedControlDocument(p.Document, p.Digest); e != nil {
			return fail("STORAGE_BUDGET_SNAPSHOT_UNCONFIRMED")
		}
		if _, e = verifiedControlDocument(revision.Document, revision.Digest); e != nil {
			return fail("STORAGE_BUDGET_SNAPSHOT_UNCONFIRMED")
		}
		applied, e := strictHostBudget(p.Document)
		if e != nil || applied.AuthorityID != d.AuthorityID || applied.Origin != d.Origin {
			return fail("STORAGE_BUDGET_SNAPSHOT_UNCONFIRMED")
		}
		effective = applied.Settings.policy(p.Revision)
		out.SnapshotRevision = &p.Revision
	}
	// 未发布的放宽值不能扩大准入；收紧值立即进入数据库锁保护的保守合并。
	effective = conservativeBudget(effective, d.Settings.policy(max(1, state.AppliedRevision)))
	value := budgetSettings(effective)
	out.EffectivePolicy = &value
	out.PendingProtection = state.AppliedRevision != state.DesiredRevision
	return out, state, effective, nil
}
func (s *Service) fenceHostBudget(ctx context.Context, snapshot budget.ControlSnapshot, action func(budget.BudgetPolicy) error) error {
	run, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, state, p, e := s.loadHostBudget(run, snapshot)
	if e != nil {
		return e
	}
	c := s.ownerBudgetControl(snapshot)
	return c.repo.WithCurrent(run, c.opts.Scope.ID(), string(ControlBudget), func(row *m.ControlConfiguration) error {
		if state.State == "unconfigured" {
			if row != nil {
				return budget.ErrControlChanged
			}
			return action(p)
		}
		if row == nil || row.Revision != state.DesiredRevision || row.AppliedRevision != state.AppliedRevision || row.Digest != state.Digest || row.HostID != c.opts.Scope.HostID || row.FilesystemID != c.opts.Scope.FilesystemID || row.InstanceID != c.opts.Scope.InstanceID {
			return budget.ErrControlChanged
		}
		if _, e := verifiedControlDocument(row.Document, row.Digest); e != nil {
			return e
		}
		return action(p)
	})
}
func (s *Service) HostBudgetConfiguration(ctx context.Context) (HostBudgetConfiguration, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return HostBudgetConfiguration{}, ErrRootRequired
	}
	if s.opts.Budget == nil {
		return HostBudgetConfiguration{}, ErrStateUnavailable
	}
	snapshot, e := s.opts.Budget.ControlSnapshot(ctx)
	if e != nil {
		return HostBudgetConfiguration{Authority: "unavailable", State: "unavailable", Problem: "STORAGE_BUDGET_SNAPSHOT_UNAVAILABLE"}, e
	}
	out, _, _, e := s.loadHostBudget(ctx, snapshot)
	return out, e
}
func (s *Service) hostBudgetWritable(ctx context.Context) (HostBudgetConfiguration, error) {
	out, e := s.HostBudgetConfiguration(ctx)
	if e != nil {
		return out, e
	}
	if !out.CanManage {
		return out, controlError(409, "STORAGE_BUDGET_OWNER_REQUIRED")
	}
	return out, nil
}
func (s *Service) AdoptHostBudget(ctx context.Context, in HostBudgetAdoption) (HostBudgetMutation, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return HostBudgetMutation{}, ErrRootRequired
	}
	if !in.Confirm || !validTargetUUID(in.RequestID) || in.LegacyRevision < 1 {
		return HostBudgetMutation{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	if old, e := s.HostBudgetRequest(ctx, in.RequestID); e == nil {
		if old.OperationRevision != 1 || in.LegacyRevision != old.Configuration.LegacyRevision || in.LegacyDigest != old.Configuration.LegacyDigest {
			return old, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
		}
		return old, nil
	} else if !errors.Is(e, gorm.ErrRecordNotFound) && dto.CodeOf(e) != "STORAGE_CONTROL_REQUEST_NOT_FOUND" {
		return old, e
	}
	config, e := s.hostBudgetWritable(ctx)
	if e != nil {
		return HostBudgetMutation{}, e
	}
	if config.Authority != "legacy" || in.LegacyRevision != config.LegacyRevision || in.LegacyDigest != config.LegacyDigest {
		return HostBudgetMutation{}, controlError(409, "STORAGE_BUDGET_LEGACY_CHANGED")
	}
	snap, e := s.opts.Budget.ControlSnapshot(ctx)
	if e != nil {
		return HostBudgetMutation{}, e
	}
	d := hostBudgetDocument{nil, 1, in.RequestID, budgetSettings(snap.Legacy), snap.Legacy, config.LegacyDigest}
	return s.commitHostBudget(ctx, 0, in.RequestID, d)
}
func (s *Service) SaveHostBudget(ctx context.Context, in HostBudgetChange) (HostBudgetMutation, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return HostBudgetMutation{}, ErrRootRequired
	}
	if !in.Confirm || !validTargetUUID(in.RequestID) || in.ExpectedRevision < 1 || !budget.ValidPolicy(in.Settings.policy(1)) {
		return HostBudgetMutation{}, controlError(400, "STORAGE_BUDGET_SETTINGS_INVALID")
	}
	if old, e := s.HostBudgetRequest(ctx, in.RequestID); e == nil {
		if old.OperationRevision != in.ExpectedRevision+1 || old.OperationPolicy != in.Settings {
			return old, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
		}
		return old, nil
	} else if dto.CodeOf(e) != "STORAGE_CONTROL_REQUEST_NOT_FOUND" {
		return old, e
	}
	config, e := s.hostBudgetWritable(ctx)
	if e != nil {
		return HostBudgetMutation{}, e
	}
	if config.Authority != "database" {
		return HostBudgetMutation{}, controlError(428, "STORAGE_BUDGET_ADOPTION_REQUIRED")
	}
	if config.DesiredRevision != in.ExpectedRevision {
		return HostBudgetMutation{}, controlError(409, "STORAGE_CONTROL_REVISION_CONFLICT")
	}
	state, e := s.opts.Control.read(ctx, ControlBudget)
	if e != nil {
		return HostBudgetMutation{}, e
	}
	d, e := strictHostBudget(state.Document)
	if e != nil {
		return HostBudgetMutation{}, e
	}
	d.Settings = in.Settings
	return s.commitHostBudget(ctx, in.ExpectedRevision, in.RequestID, d)
}
func (s *Service) commitHostBudget(ctx context.Context, rev int64, key string, d hostBudgetDocument) (HostBudgetMutation, error) {
	raw, e := json.Marshal(d)
	if e != nil {
		return HostBudgetMutation{}, e
	}
	state, e := s.opts.Control.Save(ctx, ControlInput{key, rev, ControlBudget, raw})
	if e != nil {
		return HostBudgetMutation{}, e
	}
	out, _ := s.HostBudgetConfiguration(ctx)
	return HostBudgetMutation{key, state.OperationRevision, d.Settings, out}, nil
}
func (s *Service) HostBudgetRequest(ctx context.Context, key string) (HostBudgetMutation, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return HostBudgetMutation{}, ErrRootRequired
	}
	if !validTargetUUID(key) {
		return HostBudgetMutation{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	if s.opts.Control == nil {
		return HostBudgetMutation{}, ErrStateUnavailable
	}
	row, e := s.opts.Control.repo.Request(ctx, s.opts.Control.opts.Scope.ID(), key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return HostBudgetMutation{}, controlError(404, "STORAGE_CONTROL_REQUEST_NOT_FOUND")
	}
	if e != nil {
		return HostBudgetMutation{}, controlError(503, "STORAGE_CONTROL_UNAVAILABLE")
	}
	if row.Domain != string(ControlBudget) || row.ActorID != reqctx.GetClaims(ctx).UserID {
		return HostBudgetMutation{}, controlError(403, "STORAGE_CONTROL_REQUEST_FORBIDDEN")
	}
	if _, e = verifiedControlDocument(row.Document, row.Digest); e != nil {
		return HostBudgetMutation{}, e
	}
	d, e := strictHostBudget(row.Document)
	if e != nil {
		return HostBudgetMutation{}, e
	}
	out, _ := s.HostBudgetConfiguration(ctx)
	return HostBudgetMutation{key, row.Revision, d.Settings, out}, nil
}
