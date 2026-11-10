package storagegov

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	storagerecovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
)

// ExecutionChange 是固定操作合同，不接受域名、表名、路径或任意执行载荷。
type ExecutionChange struct {
	RequestID             string `json:"request_id"`
	ExpectedRevision      int64  `json:"expected_revision"`
	Action                string `json:"action"`
	Reason                string `json:"reason"`
	Confirm               bool   `json:"confirm"`
	ExternalStopped       bool   `json:"external_stopped"`
	AdmittedTasksChecked  bool   `json:"admitted_tasks_checked"`
	ExpectedRecoveryEpoch string `json:"expected_recovery_epoch,omitempty"`
	EnvironmentChecked    bool   `json:"environment_checked,omitempty"`
}
type executionDocument struct {
	PreparedRecoveryEpoch string          `json:"prepared_recovery_epoch,omitempty"`
	Version               int             `json:"version"`
	Mode                  string          `json:"mode"`
	ReviewPolicies        bool            `json:"review_policies"`
	Operation             ExecutionChange `json:"operation"`
	RecoveryEpoch         string          `json:"recovery_epoch,omitempty"`
}
type executionBinding struct {
	Version  int    `json:"version"`
	Scope    string `json:"scope"`
	Revision int64  `json:"revision"`
	Digest   string `json:"digest"`
}
type ExecutionPolicyStatus struct {
	Domain       string `json:"domain"`
	Kind         string `json:"kind"`
	ResourceName string `json:"resource_name,omitempty"`
	Paused       bool   `json:"paused"`
	Confirmed    bool   `json:"confirmed"`
	Problem      string `json:"problem,omitempty"`
}
type ExecutionConfiguration struct {
	LegacyHandoffAvailable bool                    `json:"legacy_handoff_available"`
	Mode                   string                  `json:"mode"`
	DesiredRevision        int64                   `json:"desired_revision"`
	AppliedRevision        int64                   `json:"applied_revision"`
	Confirmed              bool                    `json:"confirmed"`
	CanPause               bool                    `json:"can_pause"`
	CanResume              bool                    `json:"can_resume"`
	WorkerReady            bool                    `json:"worker_ready"`
	Policies               []ExecutionPolicyStatus `json:"policies"`
	Problem                string                  `json:"problem,omitempty"`
	Recovery               ExecutionRecovery       `json:"recovery"`
}
type ExecutionMutation struct {
	RequestID         string                 `json:"request_id"`
	OperationRevision int64                  `json:"operation_revision"`
	Operation         ExecutionChange        `json:"operation"`
	Configuration     ExecutionConfiguration `json:"configuration"`
}

func decodeExecution(raw []byte) (executionDocument, error) {
	var d executionDocument
	x := json.NewDecoder(bytes.NewReader(raw))
	x.DisallowUnknownFields()
	o := &d.Operation
	if x.Decode(&d) != nil || x.Decode(new(any)) != io.EOF || d.Version != 1 || !validTargetUUID(o.RequestID) || o.ExpectedRevision < 0 || !o.Confirm || utf8.RuneCountInString(strings.TrimSpace(o.Reason)) < 3 || utf8.RuneCountInString(o.Reason) > 500 {
		return d, controlError(400, "STORAGE_EXECUTION_DOCUMENT_INVALID")
	}
	if d.RecoveryEpoch != "" && !validTargetUUID(d.RecoveryEpoch) || d.PreparedRecoveryEpoch != "" && !validTargetUUID(d.PreparedRecoveryEpoch) || o.Action != "acknowledge_restore" && o.Action != "prepare_restore" && (o.ExpectedRecoveryEpoch != "" || o.EnvironmentChecked) {
		return d, controlError(400, "STORAGE_EXECUTION_DOCUMENT_INVALID")
	}
	switch o.Action {
	case "prepare_restore":
		if d.Mode != "paused" || !d.ReviewPolicies || !validTargetUUID(o.ExpectedRecoveryEpoch) || d.PreparedRecoveryEpoch != o.ExpectedRecoveryEpoch || !o.ExternalStopped || !o.AdmittedTasksChecked || !o.EnvironmentChecked {
			return d, controlError(400, "STORAGE_EXECUTION_DOCUMENT_INVALID")
		}
	case "pause":
		if d.Mode != "paused" {
			return d, controlError(400, "STORAGE_EXECUTION_DOCUMENT_INVALID")
		}
	case "resume":
		if d.Mode != "open" || d.ReviewPolicies {
			return d, controlError(400, "STORAGE_EXECUTION_DOCUMENT_INVALID")
		}
	case "begin_handoff":
		if d.Mode != "handoff" || !d.ReviewPolicies {
			return d, controlError(400, "STORAGE_EXECUTION_DOCUMENT_INVALID")
		}
	case "complete_handoff":
		if d.Mode != "paused" || !d.ReviewPolicies || !o.ExternalStopped || !o.AdmittedTasksChecked {
			return d, controlError(400, "STORAGE_EXECUTION_DOCUMENT_INVALID")
		}
	case "acknowledge_restore":
		if d.Mode != "paused" || !d.ReviewPolicies || !validTargetUUID(o.ExpectedRecoveryEpoch) || d.RecoveryEpoch != o.ExpectedRecoveryEpoch || !o.ExternalStopped || !o.AdmittedTasksChecked || !o.EnvironmentChecked {
			return d, controlError(400, "STORAGE_EXECUTION_DOCUMENT_INVALID")
		}
	default:
		return d, controlError(400, "STORAGE_EXECUTION_DOCUMENT_INVALID")
	}
	return d, nil
}

func executionDocumentMatches(raw []byte, digest string) bool {
	_, e := verifiedControlDocument(raw, digest)
	return e == nil
}

func executionRevisionMatches(d executionDocument, revision int64) bool {
	if d.Operation.Action == "prepare_restore" {
		return revision > d.Operation.ExpectedRevision
	}
	return revision == d.Operation.ExpectedRevision+1
}

func (s *Service) bindExecutionControl() error {
	if s.opts.Budget == nil {
		return nil
	}
	ctrl := s.opts.Control
	ctrl.executionGuard = s.withRecoveryGuard
	if e := ctrl.Bind(ControlExecution, ControlBinding{
		Validate: func(raw json.RawMessage) error { _, e := decodeExecution(raw); return e },
		Commit:   s.commitExecution,
		Apply: func(ctx context.Context, p ControlPublication) error {
			if p.Scope != ctrl.opts.Scope || p.Domain != ControlExecution {
				return ErrStateUnavailable
			}
			if _, e := decodeExecution(p.Document); e != nil {
				return e
			}
			if e := s.markExecution(ctx, p.Revision, p.Digest); e != nil {
				return e
			}
			_, e := updateArchiveControlProjection(ctx, s.opts.Budget, "execution-control-snapshot", func(raw []byte) ([]byte, error) {
				if len(raw) > 0 {
					var old ControlPublication
					if json.Unmarshal(raw, &old) != nil || old.Scope != p.Scope || old.Domain != p.Domain || old.Revision > p.Revision || old.Revision == p.Revision && old.Digest != p.Digest {
						return nil, ErrStateUnavailable
					}
				}
				return json.Marshal(p)
			})
			return e
		},
	}); e != nil {
		return e
	}
	_, e := updateArchiveControlProjection(context.Background(), s.opts.Budget, "execution-control-binding", func(raw []byte) ([]byte, error) {
		if len(raw) == 0 {
			return json.Marshal(executionBinding{Version: 1, Scope: ctrl.opts.Scope.ID()})
		}
		var b executionBinding
		if json.Unmarshal(raw, &b) != nil || b.Version != 1 || b.Scope != ctrl.opts.Scope.ID() || b.Revision < 0 || b.Revision > 0 && len(b.Digest) != 64 {
			return nil, ErrStateUnavailable
		}
		return nil, nil
	})
	if e != nil {
		return e
	}
	ctrl.executionFence = func(ctx context.Context, tx *gorm.DB, row *m.ControlConfiguration) error {
		c, _, e := s.executionStateTx(ctx, tx, row)
		if e != nil {
			return e
		}
		if c.Mode == "unmanaged" || c.Mode == "open" && c.Confirmed && c.WorkerReady {
			return nil
		}
		return controlError(423, "STORAGE_EXECUTION_PAUSED")
	}
	return nil
}

// markExecution 对已确认的暂停保留主机水位；即使发布消费者退出也不退回旧开关。
func (s *Service) markExecution(ctx context.Context, revision int64, digest string) error {
	_, e := updateArchiveControlProjection(ctx, s.opts.Budget, "execution-control-binding", func(raw []byte) ([]byte, error) {
		var b executionBinding
		if json.Unmarshal(raw, &b) != nil || b.Version != 1 || b.Scope != s.opts.Control.opts.Scope.ID() || b.Revision < 0 || b.Revision > 0 && len(b.Digest) != 64 || b.Revision == revision && b.Digest != digest {
			return nil, ErrStateUnavailable
		}
		if b.Revision > revision {
			return nil, nil
		}
		return json.Marshal(executionBinding{1, b.Scope, revision, digest})
	})
	return e
}

// executionStateTx 只使用当前事务的连接；主机文档锁不包含 SQL 回调。
func (s *Service) executionStateTx(ctx context.Context, tx *gorm.DB, row *m.ControlConfiguration) (ExecutionConfiguration, executionDocument, error) {
	return s.executionStateTxPinned(ctx, tx, row, nil)
}
func (s *Service) executionStateTxPinned(ctx context.Context, tx *gorm.DB, row *m.ControlConfiguration, epoch *storagerecovery.Epoch) (ExecutionConfiguration, executionDocument, error) {
	c := ExecutionConfiguration{LegacyHandoffAvailable: !s.opts.LegacyHandoffBlocked, Mode: "unavailable", Policies: []ExecutionPolicyStatus{}, WorkerReady: s.opts.ControlWorkerEnabled && !s.opts.Control.ConsumerExited()}
	d := executionDocument{}
	fail := func() (ExecutionConfiguration, executionDocument, error) {
		c.Problem = "STORAGE_EXECUTION_UNCONFIRMED"
		return c, d, controlError(503, c.Problem)
	}
	raw, e := readArchiveControlProjection(ctx, s.opts.Budget, "execution-control-binding")
	var b executionBinding
	if e != nil || json.Unmarshal(raw, &b) != nil || b.Version != 1 || b.Scope != s.opts.Control.opts.Scope.ID() || b.Revision < 0 || b.Revision > 0 && len(b.Digest) != 64 {
		return fail()
	}
	raw, e = readArchiveControlProjection(ctx, s.opts.Budget, "execution-control-snapshot")
	if e != nil {
		return fail()
	}
	c.CanPause = true
	if row == nil {
		if b.Revision != 0 || len(raw) > 0 {
			return fail()
		}
		c.Mode = "unmanaged"
		return s.finishRecoveryStatePinned(ctx, c, d, epoch)
	}
	c.DesiredRevision, c.AppliedRevision = row.Revision, row.AppliedRevision
	if row.HostID != s.opts.Control.opts.Scope.HostID || row.FilesystemID != s.opts.Control.opts.Scope.FilesystemID || row.InstanceID != s.opts.Control.opts.Scope.InstanceID || row.Revision < 1 || row.Revision < b.Revision || row.Revision == b.Revision && row.Digest != b.Digest {
		return fail()
	}
	if _, e = verifiedControlDocument(row.Document, row.Digest); e != nil {
		return fail()
	}
	d, e = decodeExecution(row.Document)
	if e != nil || !executionRevisionMatches(d, row.Revision) {
		return fail()
	}
	v, e := s.opts.Control.repo.RevisionTx(ctx, tx, row.ScopeID, row.Domain, row.Revision)
	if e != nil || v.Digest != row.Digest || v.ActorID != row.ActorID || !bytes.Equal(v.Document, row.Document) || v.RequestID != d.Operation.RequestID {
		return fail()
	}
	c.Mode = d.Mode
	var p ControlPublication
	c.Confirmed = row.PublicationState == "applied" && row.AppliedRevision == row.Revision && b.Revision == row.Revision && b.Digest == row.Digest && json.Unmarshal(raw, &p) == nil && p.Scope == s.opts.Control.opts.Scope && p.ScopeID == row.ScopeID && p.Domain == ControlExecution && p.Revision == row.Revision && p.Digest == row.Digest && p.ActorID == row.ActorID && p.UpdatedAt.Equal(v.CreatedAt) && executionDocumentMatches(p.Document, v.Digest)
	c.CanResume = c.Mode == "paused" && c.Confirmed && c.WorkerReady
	if !c.Confirmed {
		c.Problem = "STORAGE_EXECUTION_PUBLICATION_PENDING"
	}
	return s.finishRecoveryStatePinned(ctx, c, d, epoch)
}

// policyPaused 固定白名单、固定 DTO；保护／观测配置不要求关闭。
func (s *Service) policyPaused(row m.ControlConfiguration) (bool, bool) {
	domain := ControlDomain(row.Domain)
	switch domain {
	case ControlExecution, ControlBudget, ControlArchiveTargets, ControlTableMonitoring:
		return true, false
	}
	b, ok := s.opts.Control.binding(domain)
	if !ok || b.Validate(json.RawMessage(row.Document)) != nil {
		return false, true
	}
	if _, e := verifiedControlDocument(row.Document, row.Digest); e != nil {
		return false, true
	}
	switch {
	case strings.HasPrefix(row.Domain, "file-lifecycle."):
		d, e := decodeLifecycleDocument(row.Document)
		return e == nil && !d.Settings.Enabled, true
	case domain == ControlArchivePolicies:
		d, e := decodeArchivePolicies(row.Document)
		if e != nil {
			return false, true
		}
		for _, p := range d.Entries {
			if p.Enabled || p.RemoteCleanupEnabled {
				return false, true
			}
		}
		return true, true
	case domain == ControlBackupPolicies:
		var d backupPolicyDocument
		if json.Unmarshal(row.Document, &d) != nil {
			return false, true
		}
		for _, p := range d.Policies {
			if p.Enabled || p.Settings.DrillEnabled {
				return false, true
			}
		}
		return true, true
	case domain == ControlSnapshotPolicies:
		d, e := decodeSnapshotPolicies(row.Document)
		if e != nil {
			return false, true
		}
		for _, p := range d.Policies {
			if p.Enabled {
				return false, true
			}
		}
		return true, true
	case domain == ControlDatabaseRetention:
		d, e := decodeRetentionControl(row.Document)
		return e == nil && !d.Enabled, true
	default:
		for _, id := range []string{repo.ProbeRetentionID, repo.DiscoveryRetentionID} {
			if domain == systemRetentionDomain(id) {
				d, e := decodeSystemRetentionControl(row.Document, id)
				return e == nil && !d.Enabled, true
			}
		}
		return false, true
	}
}
func (s *Service) executionPoliciesTx(ctx context.Context, tx *gorm.DB) ([]ExecutionPolicyStatus, error) {
	rows, e := s.opts.Control.repo.ConfigurationsTx(ctx, tx, s.opts.Control.opts.Scope.ID())
	if e != nil {
		return nil, e
	}
	out := []ExecutionPolicyStatus{}
	for _, row := range rows {
		paused, include := s.policyPaused(row)
		if !include {
			continue
		}
		v, err := s.opts.Control.repo.RevisionTx(ctx, tx, row.ScopeID, row.Domain, row.Revision)
		valid := err == nil && v.Digest == row.Digest && bytes.Equal(v.Document, row.Document) && v.ActorID == row.ActorID
		kind, name := s.executionPolicyLabel(row)
		p := ExecutionPolicyStatus{Domain: row.Domain, Kind: kind, ResourceName: name, Paused: paused && valid, Confirmed: valid && row.AppliedRevision == row.Revision && row.PublicationState == "applied"}
		if !p.Paused || !p.Confirmed {
			p.Problem = "STORAGE_EXECUTION_POLICY_NOT_PAUSED"
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Service) executionPolicyLabel(row m.ControlConfiguration) (string, string) {
	switch ControlDomain(row.Domain) {
	case ControlArchivePolicies:
		return "archives", ""
	case ControlBackupPolicies:
		return "backups", ""
	case ControlSnapshotPolicies:
		return "snapshots", ""
	case ControlDatabaseRetention:
		return "invocations", ""
	}
	if ControlDomain(row.Domain) == systemRetentionDomain(repo.ProbeRetentionID) {
		return "probes", ""
	}
	if ControlDomain(row.Domain) == systemRetentionDomain(repo.DiscoveryRetentionID) {
		return "discovery", ""
	}
	if strings.HasPrefix(row.Domain, "file-lifecycle.") {
		if d, e := decodeLifecycleDocument(row.Document); e == nil && s.opts.Lifecycle != nil {
			if source, ok := s.opts.Lifecycle.Source(d.SourceID); ok {
				return "file_logs", source.Name
			}
		}
		return "file_logs", ""
	}
	return "unknown", ""
}

func (s *Service) commitExecution(ctx context.Context, tx *gorm.DB, raw json.RawMessage) error {
	d, e := decodeExecution(raw)
	if e != nil {
		return e
	}
	o := d.Operation
	if s.opts.LegacyHandoffBlocked && (o.Action == "begin_handoff" || o.Action == "complete_handoff") {
		return controlError(423, "STORAGE_HANDOFF_RELEASE_NOT_ACCEPTED")
	}

	var prev executionDocument
	if o.ExpectedRevision > 0 {
		v, e := s.opts.Control.repo.RevisionTx(ctx, tx, s.opts.Control.opts.Scope.ID(), string(ControlExecution), o.ExpectedRevision)
		if e != nil {
			return ErrStateUnavailable
		}
		if _, e = verifiedControlDocument(v.Document, v.Digest); e != nil {
			return e
		}
		prev, e = decodeExecution(v.Document)
		if e != nil {
			return e
		}
	}
	if o.Action == "acknowledge_restore" {
		r, e := s.recoveryStatus(ctx)
		if e != nil {
			return e
		}
		if r.Epoch == "" || r.Epoch != o.ExpectedRecoveryEpoch || prev.RecoveryEpoch == r.Epoch || !s.opts.ControlWorkerEnabled || s.opts.Control.ConsumerExited() {
			return controlError(409, "STORAGE_RECOVERY_EPOCH_CHANGED")
		}
		if prev.PreparedRecoveryEpoch != r.Epoch {
			return controlError(409, "STORAGE_RECOVERY_REBUILD_REQUIRED")
		}
		current, e := s.opts.Control.repo.GetTx(ctx, tx, s.opts.Control.opts.Scope.ID(), string(ControlExecution))
		if e != nil || current.AppliedRevision != o.ExpectedRevision {
			return controlError(409, "STORAGE_EXECUTION_PUBLICATION_PENDING")
		}
		previousVersion, e := s.opts.Control.repo.RevisionTx(ctx, tx, current.ScopeID, current.Domain, o.ExpectedRevision)
		if e != nil {
			return e
		}
		previous := *current
		previous.Revision, previous.Document, previous.Digest = previousVersion.Revision, previousVersion.Document, previousVersion.Digest
		if e = s.checkRecoveryProjection(ctx, previous); e != nil {
			return controlError(409, "STORAGE_EXECUTION_PUBLICATION_PENDING")
		}
		resources, e := s.recoveryResourcesTx(ctx, tx, r.Epoch)
		if e != nil {
			return e
		}
		for _, resource := range resources {
			if resource.State != "ready" {
				return controlError(409, resource.Problem)
			}
		}
		policies, e := s.executionPoliciesTx(ctx, tx)
		if e != nil {
			return e
		}
		for _, p := range policies {
			if !p.Paused || !p.Confirmed {
				return controlError(409, "STORAGE_EXECUTION_POLICY_NOT_PAUSED")
			}
		}
		return nil
	}
	r, e := s.recoveryStatus(ctx)
	if e != nil {
		return e
	}
	if (o.Action == "resume" || o.Action == "complete_handoff") && r.Epoch != "" && prev.RecoveryEpoch != r.Epoch {
		return controlError(409, "STORAGE_RECOVERY_REVIEW_REQUIRED")
	}
	if o.Action == "pause" {
		if prev.Mode == "handoff" || prev.ReviewPolicies != d.ReviewPolicies {
			return controlError(409, "STORAGE_EXECUTION_HANDOFF_LOCKED")
		}
		return nil
	}
	if o.Action == "begin_handoff" {
		return nil
	}
	// 当前行已在 CAS 中更新；只读核对上一已发布版本，绝不在此启动执行器。
	current, e := s.opts.Control.repo.GetTx(ctx, tx, s.opts.Control.opts.Scope.ID(), string(ControlExecution))
	if e != nil || current.AppliedRevision != o.ExpectedRevision || o.ExpectedRevision < 1 || !s.opts.ControlWorkerEnabled || s.opts.Control.ConsumerExited() {
		return controlError(409, "STORAGE_EXECUTION_PUBLICATION_PENDING")
	}
	v, e := s.opts.Control.repo.RevisionTx(ctx, tx, current.ScopeID, current.Domain, o.ExpectedRevision)
	if e != nil {
		return e
	}
	previous := *current
	previous.Revision = v.Revision
	previous.Document = v.Document
	previous.Digest = v.Digest
	previous.ActorID = v.ActorID
	previous.PublicationState = "applied"
	c, _, e := s.executionStateTx(ctx, tx, &previous)
	if e != nil {
		return e
	}
	if !c.Confirmed {
		return controlError(409, "STORAGE_EXECUTION_PUBLICATION_PENDING")
	}
	if o.Action == "complete_handoff" && prev.Mode != "handoff" || o.Action == "resume" && prev.Mode != "paused" {
		return controlError(409, "STORAGE_EXECUTION_TRANSITION_INVALID")
	}
	if o.Action == "complete_handoff" || prev.ReviewPolicies {
		policies, e := s.executionPoliciesTx(ctx, tx)
		if e != nil {
			return e
		}
		for _, p := range policies {
			if !p.Paused || !p.Confirmed {
				return controlError(409, "STORAGE_EXECUTION_POLICY_NOT_PAUSED")
			}
		}
	}
	return nil
}

func (s *Service) executionAvailable(ctx context.Context) error {
	if !auth.IsPlatformRootSession(ctx) {
		return ErrRootRequired
	}
	if s.opts.DB == nil || s.opts.Control == nil || s.opts.Budget == nil || !s.opts.Control.Ready(ctx) {
		return controlError(503, "STORAGE_CONTROL_SCHEMA_NOT_READY")
	}
	return nil
}
func (s *Service) ExecutionConfiguration(ctx context.Context) (ExecutionConfiguration, error) {
	c := ExecutionConfiguration{LegacyHandoffAvailable: !s.opts.LegacyHandoffBlocked, Mode: "unavailable", Policies: []ExecutionPolicyStatus{}}
	if e := s.executionAvailable(ctx); e != nil {
		return c, e
	}
	e := s.withRecoveryGuard(ctx, func() error {
		return s.opts.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return s.opts.Control.repo.WithExecutionTx(ctx, tx, s.opts.Control.opts.Scope.ID(), func(row *m.ControlConfiguration) error {
				var e error
				var d executionDocument
				c, d, e = s.executionStateTx(ctx, tx, row)
				if e != nil {
					// 恢复后的数据库可能落后于独立水位。保留阻断，同时允许
					// Root 看到当前 CAS 版本并显式重建，不能删水位绕过。
					r, re := s.recoveryStatus(ctx)
					if re != nil || r.Epoch == "" {
						return e
					}
					c = ExecutionConfiguration{Mode: "recovery", Recovery: r, Policies: []ExecutionPolicyStatus{}, WorkerReady: s.opts.ControlWorkerEnabled && !s.opts.Control.ConsumerExited(), Problem: "STORAGE_RECOVERY_REBUILD_REQUIRED"}
					if row != nil {
						c.DesiredRevision = row.Revision
						c.AppliedRevision = row.AppliedRevision
					}
				}
				if c.Recovery.Epoch != "" && c.Recovery.State != "acknowledged" {
					c.Recovery.Prepared = d.PreparedRecoveryEpoch == c.Recovery.Epoch
					c.Recovery.Resources, e = s.recoveryResourcesTx(ctx, tx, c.Recovery.Epoch)
					if e != nil {
						return e
					}
				}
				c.Policies, e = s.executionPoliciesTx(ctx, tx)
				if d.ReviewPolicies {
					for _, p := range c.Policies {
						if !p.Paused || !p.Confirmed {
							c.CanResume = false
						}
					}
				}
				return e
			})
		})
	})
	return c, e
}
func (s *Service) ChangeExecution(ctx context.Context, in ExecutionChange) (ExecutionMutation, error) {
	if e := s.executionAvailable(ctx); e != nil {
		return ExecutionMutation{}, e
	}
	if !validTargetUUID(in.RequestID) || in.ExpectedRevision < 0 || !in.Confirm {
		return ExecutionMutation{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	// 原 UUID 的不可变回执优先；不会把稍后的当前版本冒充此操作。
	if v, e := s.opts.Control.repo.Request(ctx, s.opts.Control.opts.Scope.ID(), in.RequestID); e == nil {
		d, e := decodeExecution(v.Document)
		if e != nil || v.Domain != string(ControlExecution) || d.Operation != in || v.ActorID != reqctx.GetClaims(ctx).UserID {
			return ExecutionMutation{}, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
		}
		return s.ExecutionRequest(ctx, in.RequestID)
	} else if !errors.Is(e, gorm.ErrRecordNotFound) {
		return ExecutionMutation{}, e
	}
	if s.opts.LegacyHandoffBlocked && (in.Action == "begin_handoff" || in.Action == "complete_handoff") {
		return ExecutionMutation{}, controlError(423, "STORAGE_HANDOFF_RELEASE_NOT_ACCEPTED")
	}
	if in.Action == "prepare_restore" {
		return s.prepareRestore(ctx, in)
	}
	d := executionDocument{Version: 1, Mode: "paused", Operation: in}
	if in.ExpectedRevision > 0 {
		v, e := s.opts.Control.repo.Revision(ctx, s.opts.Control.opts.Scope.ID(), string(ControlExecution), in.ExpectedRevision)
		if e != nil {
			return ExecutionMutation{}, controlError(409, "STORAGE_CONTROL_REVISION_CONFLICT")
		}
		old, e := decodeExecution(v.Document)
		if e != nil {
			return ExecutionMutation{}, e
		}
		d.ReviewPolicies = old.ReviewPolicies
		d.RecoveryEpoch = old.RecoveryEpoch
		d.PreparedRecoveryEpoch = old.PreparedRecoveryEpoch
	}
	switch in.Action {
	case "resume":
		d.Mode = "open"
		d.ReviewPolicies = false
	case "begin_handoff":
		d.Mode = "handoff"
		d.ReviewPolicies = true
	case "complete_handoff":
		d.ReviewPolicies = true
	case "acknowledge_restore":
		d.ReviewPolicies = true
		d.RecoveryEpoch = in.ExpectedRecoveryEpoch
	}
	raw, _ := json.Marshal(d)
	out, e := s.opts.Control.Save(ctx, ControlInput{RequestID: in.RequestID, ExpectedRevision: in.ExpectedRevision, Domain: ControlExecution, Document: raw})
	if e != nil {
		return ExecutionMutation{}, e
	}
	// 暂停不依赖消费者确认；成功答复前留下独立主机水位。失败答复按原 UUID 查询。
	if d.Mode != "open" {
		body, _ := canonicalControl(raw)
		if e = s.markExecution(ctx, out.OperationRevision, controlDigest(body)); e != nil {
			return ExecutionMutation{}, e
		}
	}
	return s.ExecutionRequest(ctx, in.RequestID)
}
func (s *Service) ExecutionRequest(ctx context.Context, id string) (ExecutionMutation, error) {
	if e := s.executionAvailable(ctx); e != nil {
		return ExecutionMutation{}, e
	}
	if !validTargetUUID(id) {
		return ExecutionMutation{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	v, e := s.opts.Control.repo.Request(ctx, s.opts.Control.opts.Scope.ID(), id)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return ExecutionMutation{}, controlError(404, "STORAGE_CONTROL_REQUEST_NOT_FOUND")
	}
	if e != nil {
		return ExecutionMutation{}, e
	}
	if v.Domain != string(ControlExecution) {
		return ExecutionMutation{}, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
	}
	if _, e = verifiedControlDocument(v.Document, v.Digest); e != nil {
		return ExecutionMutation{}, e
	}
	d, e := decodeExecution(v.Document)
	if e != nil || d.Operation.RequestID != id || !executionRevisionMatches(d, v.Revision) {
		return ExecutionMutation{}, ErrStateUnavailable
	}
	c, e := s.ExecutionConfiguration(ctx)
	if e != nil {
		c = ExecutionConfiguration{Mode: "unavailable", Policies: []ExecutionPolicyStatus{}, Problem: "STORAGE_EXECUTION_UNCONFIRMED", Recovery: ExecutionRecovery{State: "unavailable"}}
	}
	return ExecutionMutation{RequestID: id, OperationRevision: v.Revision, Operation: d.Operation, Configuration: c}, nil
}
