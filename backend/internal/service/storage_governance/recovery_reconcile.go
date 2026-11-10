package storagegov

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// RecoveryStamp 保留被恢复配置的来源版本；恢复产生新审计，不覆盖历史回执。
type RecoveryStamp struct {
	Epoch            string `json:"epoch"`
	RequestID        string `json:"request_id"`
	PreviousRevision int64  `json:"previous_revision"`
	PreviousDigest   string `json:"previous_digest"`
	Environment      string `json:"environment"`
}
type RecoveryResource struct {
	Domain   string `json:"domain"`
	Kind     string `json:"kind"`
	Revision int64  `json:"revision"`
	State    string `json:"state"`
	Problem  string `json:"problem,omitempty"`
}

func (s *Service) recoveryEnvironment() string {
	return controlDigest([]byte(s.opts.Control.opts.Scope.ID() + "\x00" + s.opts.Identity.Environment + "\x00" + s.retentionSource()))
}
func recoveryStamp(raw []byte) *RecoveryStamp {
	var d struct {
		Recovery *RecoveryStamp `json:"recovery"`
	}
	if json.Unmarshal(raw, &d) != nil {
		return nil
	}
	return d.Recovery
}

func validRecoveryStamp(stamp *RecoveryStamp) bool {
	if stamp == nil {
		return true
	}
	_, digestErr := hex.DecodeString(stamp.PreviousDigest)
	_, environmentErr := hex.DecodeString(stamp.Environment)
	return validTargetUUID(stamp.Epoch) && validTargetUUID(stamp.RequestID) && stamp.PreviousRevision > 0 && len(stamp.PreviousDigest) == 64 && digestErr == nil && len(stamp.Environment) == 64 && environmentErr == nil
}

// 单项版本可能继承旧策略水位，不能假设它小于控制配置版本。
func recoveryEntryFloor(raw []byte) int64 {
	var d struct {
		Entries []struct {
			Revision int64 `json:"revision"`
		} `json:"entries"`
		Policies []struct {
			Revision int64 `json:"revision"`
		} `json:"policies"`
	}
	if json.Unmarshal(raw, &d) != nil {
		return 0
	}
	var floor int64
	for _, p := range d.Entries {
		floor = max(floor, p.Revision)
	}
	for _, p := range d.Policies {
		floor = max(floor, p.Revision)
	}
	return floor
}
func (s *Service) validateRecoveryRow(ctx context.Context, tx *gorm.DB, row m.ControlConfiguration) error {
	scope := s.opts.Control.opts.Scope
	if row.ScopeID != scope.ID() || row.HostID != scope.HostID || row.FilesystemID != scope.FilesystemID || row.InstanceID != scope.InstanceID || row.Revision < 1 {
		return ErrStateUnavailable
	}
	b, ok := s.opts.Control.binding(ControlDomain(row.Domain))
	if !ok || b.Validate(json.RawMessage(row.Document)) != nil || !validRecoveryStamp(recoveryStamp(row.Document)) {
		return controlError(409, "STORAGE_RECOVERY_RESOURCE_UNSUPPORTED")
	}
	if _, e := verifiedControlDocument(row.Document, row.Digest); e != nil {
		return e
	}
	v, e := s.opts.Control.repo.RevisionTx(ctx, tx, row.ScopeID, row.Domain, row.Revision)
	if e != nil || v.Digest != row.Digest || v.ActorID != row.ActorID || !bytes.Equal(v.Document, row.Document) {
		return controlError(409, "STORAGE_RECOVERY_AUDIT_MISMATCH")
	}
	return nil
}

func recoveryFiles(domain ControlDomain) []string {
	switch domain {
	case ControlExecution:
		return []string{"execution-control-binding", "execution-control-snapshot"}
	case ControlTableMonitoring:
		return []string{"table-monitoring-control"}
	case ControlDatabaseRetention:
		return []string{"database-retention-binding", "database-retention-control"}
	case ControlArchiveTargets:
		return []string{"archive-target-control"}
	case ControlArchivePolicies:
		return []string{"archive-policy-control"}
	case ControlBackupPolicies:
		return []string{"backup-policy-control"}
	case ControlSnapshotPolicies:
		return []string{"snapshot-policy-control"}
	}
	for _, id := range []string{repo.ProbeRetentionID, repo.DiscoveryRetentionID} {
		if domain == systemRetentionDomain(id) {
			a, b := systemRetentionFiles(id)
			return []string{a, b}
		}
	}
	return nil
}

// 水位只作版本下限；从不把运行目录中的正文反向当作数据库配置权威。
func (s *Service) recoveryFloor(ctx context.Context, domain ControlDomain) (int64, error) {
	var floor int64
	for _, name := range recoveryFiles(domain) {
		raw, e := readArchiveControlProjection(ctx, s.opts.Budget, name)
		if e != nil {
			return 0, e
		}
		if len(raw) == 0 {
			continue
		}
		var p struct {
			Version  int             `json:"version"`
			Scope    json.RawMessage `json:"scope"`
			Revision int64           `json:"revision"`
			Digest   string          `json:"digest"`
			Document json.RawMessage `json:"document"`
		}
		wantVersion := 1
		if name == "execution-control-snapshot" {
			wantVersion = 0
		} // 正式 ControlPublication 没有 version 字段。
		if json.Unmarshal(raw, &p) != nil || p.Version != wantVersion || p.Revision < 0 {
			return 0, ErrStateUnavailable
		}
		var scope ControlScope
		var id string
		if json.Unmarshal(p.Scope, &id) == nil {
			if id != s.opts.Control.opts.Scope.ID() {
				return 0, ErrStateUnavailable
			}
		} else if json.Unmarshal(p.Scope, &scope) != nil || scope != s.opts.Control.opts.Scope {
			return 0, ErrStateUnavailable
		}
		if len(p.Document) > 0 {
			if _, e = verifiedControlDocument(p.Document, p.Digest); e != nil {
				return 0, e
			}
			floor = max(floor, recoveryEntryFloor(p.Document))
		}
		floor = max(floor, p.Revision)
	}
	if domain == ControlBudget {
		b, e := s.opts.Budget.ControlSnapshot(ctx)
		if e != nil {
			return 0, e
		}
		if b.Projection != nil {
			if b.Projection.OwnerInstance != s.opts.Identity.InstanceID {
				return 0, nil // 同盘预算属于另一安装，不在本安装的恢复清单内。
			}
			floor = max(floor, b.Projection.Revision)
		}
	}
	if domain == ControlSnapshotPolicies {
		if s.recoverySnapshots == nil {
			return 0, ErrStateUnavailable
		}
		b, e := s.recoverySnapshots.snapshots.ReadPolicyBinding()
		if e != nil {
			return 0, e
		}
		if b.ScopeID != s.opts.Control.opts.Scope.ID() {
			return 0, ErrStateUnavailable
		}
		floor = max(floor, b.Revision)
	}
	if strings.HasPrefix(string(domain), "file-lifecycle.") {
		if s.opts.Lifecycle == nil {
			return 0, ErrStateUnavailable
		}
		found := false
		for _, source := range s.opts.Lifecycle.RegisteredSources() {
			if lifecycleDomain(source.ID) == domain {
				found = true
				_, p, e := s.opts.Lifecycle.PolicySnapshot(ctx, source.ID)
				if e != nil {
					return 0, e
				}
				if p != nil {
					if p.InstanceID != s.opts.Identity.InstanceID || p.HostID != s.opts.Identity.HostID || p.FilesystemID != s.opts.Identity.FilesystemID {
						return 0, ErrStateUnavailable
					}
					floor = max(floor, p.Policy.Revision)
				}
			}
		}
		if !found {
			return 0, ErrStateUnavailable
		}
	}
	return floor, nil
}

// prepareRecoveryDocument 只处理正式固定 DTO。来源变化须先修复部署，不能
// 把恢复动作当成任意重绑定授权；所有执行开关清零，预算只保留或收紧。
func (s *Service) prepareRecoveryDocument(ctx context.Context, tx *gorm.DB, row m.ControlConfiguration, next int64) ([]byte, error) {
	var out any
	switch domain := ControlDomain(row.Domain); {
	case domain == ControlTableMonitoring:
		d, e := strictMonitoringDocument(row.Document)
		if e != nil {
			return nil, e
		}
		out = d
	case domain == ControlBudget:
		d, e := strictHostBudget(row.Document)
		if e != nil {
			return nil, e
		}
		snap, e := s.opts.Budget.ControlSnapshot(ctx)
		if e != nil {
			return nil, e
		}
		if snap.Projection != nil {
			if snap.Projection.OwnerInstance != s.opts.Identity.InstanceID {
				return nil, controlError(409, "STORAGE_BUDGET_OWNER_REQUIRED")
			}
			p, e := strictHostBudget(snap.Projection.Document)
			if e != nil || p.AuthorityID != d.AuthorityID {
				return nil, ErrStateUnavailable
			}
			d.Settings = budgetSettings(conservativeBudget(d.Settings.policy(next), p.Settings.policy(next)))
		}
		out = d
	case domain == ControlDatabaseRetention:
		d, e := decodeRetentionControl(row.Document)
		if e != nil {
			return nil, e
		}
		if d.Source != s.retentionSource() {
			return nil, controlError(409, "STORAGE_RECOVERY_SOURCE_CHANGED")
		}
		d.Enabled = false
		out = d
	case domain == systemRetentionDomain(repo.ProbeRetentionID) || domain == systemRetentionDomain(repo.DiscoveryRetentionID):
		id := repo.ProbeRetentionID
		if domain == systemRetentionDomain(repo.DiscoveryRetentionID) {
			id = repo.DiscoveryRetentionID
		}
		d, e := decodeSystemRetentionControl(row.Document, id)
		if e != nil {
			return nil, e
		}
		if d.Source != s.systemRetentionSource(id) {
			return nil, controlError(409, "STORAGE_RECOVERY_SOURCE_CHANGED")
		}
		d.Enabled = false
		out = d
	case strings.HasPrefix(string(domain), "file-lifecycle."):
		d, e := decodeLifecycleDocument(row.Document)
		if e != nil {
			return nil, e
		}
		source, ok := s.opts.Lifecycle.Source(d.SourceID)
		if !ok || source.Fingerprint != d.Fingerprint {
			return nil, controlError(409, "STORAGE_RECOVERY_SOURCE_CHANGED")
		}
		d.Settings.Enabled = false
		out = d
	case domain == ControlArchiveTargets:
		d, e := decodeArchiveTargets(row.Document)
		if e != nil {
			return nil, e
		}
		if s.recoveryArchives == nil {
			return nil, ErrStateUnavailable
		}
		for i := range d.Entries {
			d.Entries[i].Revision = next
		}
		out = d
	case domain == ControlArchivePolicies:
		d, e := decodeArchivePolicies(row.Document)
		if e != nil {
			return nil, e
		}
		if s.recoveryArchives == nil {
			return nil, ErrStateUnavailable
		}
		for i := range d.Entries {
			p := &d.Entries[i]
			fp, e := s.recoveryArchives.PolicySourceFingerprint(p.Settings.SourceID)
			if e != nil || fp != p.SourceFingerprint {
				return nil, controlError(409, "STORAGE_RECOVERY_SOURCE_CHANGED")
			}
			p.Revision = next
			p.Enabled = false
			p.RemoteCleanupEnabled = false
		}
		out = d
	case domain == ControlBackupPolicies:
		c := s.recoveryBackups
		if c == nil {
			return nil, ErrStateUnavailable
		}
		d, e := decodeBackupPolicies(row.Document, c.environment)
		if e != nil {
			return nil, e
		}
		rows, e := c.rows(ctx, tx)
		if e != nil {
			return nil, e
		}
		if backupEntriesDigest(entriesOfBackup(rows)) != backupEntriesDigest(d.Policies) {
			return nil, controlError(409, "BACKUP_POLICY_ROWS_CHANGED")
		}
		d.PreviousDigest = backupEntriesDigest(entriesOfBackup(rows))
		for i := range d.Policies {
			p := &d.Policies[i]
			if p.ExecutionFingerprint != c.fingerprint() {
				return nil, controlError(409, "STORAGE_RECOVERY_SOURCE_CHANGED")
			}
			p.Revision = next
			p.Enabled = false
			p.Current = false
			p.Settings.DrillEnabled = false
		}
		out = d
	case domain == ControlSnapshotPolicies:
		c := s.recoverySnapshots
		if c == nil {
			return nil, ErrStateUnavailable
		}
		d, e := decodeSnapshotPolicies(row.Document)
		if e != nil {
			return nil, e
		}
		for i := range d.Policies {
			p := &d.Policies[i]
			if p.Fingerprint != c.snapshots.PolicyFingerprint() {
				return nil, controlError(409, "STORAGE_RECOVERY_SOURCE_CHANGED")
			}
			p.Revision = next
			p.Enabled = false
			p.Current = false
		}
		out = d
	default:
		return nil, controlError(409, "STORAGE_RECOVERY_RESOURCE_UNSUPPORTED")
	}
	return json.Marshal(out)
}

func (s *Service) saveRecoveryVersion(ctx context.Context, tx *gorm.DB, old *m.ControlConfiguration, domain ControlDomain, next int64, key string, raw []byte) (*m.ControlRevision, error) {
	body, e := canonicalControl(raw)
	if e != nil {
		return nil, e
	}
	binding, ok := s.opts.Control.binding(domain)
	if !ok {
		return nil, ErrStateUnavailable
	}
	if e = binding.Validate(body); e != nil {
		return nil, e
	}
	scope := s.opts.Control.opts.Scope
	expected := int64(0)
	if old != nil {
		expected = old.Revision
	}
	actor := reqctx.GetUserID(ctx)
	digest := controlDigest(body)
	v := &m.ControlRevision{TenantID: 0, ScopeID: scope.ID(), Domain: string(domain), Revision: next, RequestID: key, RequestDigest: controlDigest([]byte(key + "\x00" + digest)), Document: datatypes.JSON(body), Digest: digest, ActorID: actor, TraceID: reqctx.GetTraceID(ctx), TaskID: uuid.NewString()}
	row := &m.ControlConfiguration{TenantID: 0, ScopeID: scope.ID(), Domain: string(domain), HostID: scope.HostID, FilesystemID: scope.FilesystemID, InstanceID: scope.InstanceID, Revision: next, Document: datatypes.JSON(body), Digest: digest, ActorID: actor, PublicationState: "pending"}
	return s.opts.Control.repo.SaveRecoveryTx(ctx, tx, row, v, expected, func(run context.Context, tx *gorm.DB) error {
		if binding.Commit != nil && domain != ControlExecution {
			if e := binding.Commit(run, tx, body); e != nil {
				return e
			}
		}
		return s.opts.Control.opts.Queue.Register(run, tx, s.opts.Control.message(v))
	})
}

func (s *Service) prepareRestore(ctx context.Context, in ExecutionChange) (ExecutionMutation, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if !validTargetUUID(in.ExpectedRecoveryEpoch) || !in.ExternalStopped || !in.AdmittedTasksChecked || !in.EnvironmentChecked {
		return ExecutionMutation{}, controlError(400, "STORAGE_EXECUTION_DOCUMENT_INVALID")
	}
	var versions []*m.ControlRevision
	e := s.withRecoveryGuard(ctx, func() error {
		return s.opts.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return s.opts.Control.repo.WithCurrentTx(ctx, tx, s.opts.Control.opts.Scope.ID(), string(ControlExecution), func(gate *m.ControlConfiguration) error {
				if existing, err := s.opts.Control.repo.RequestTx(ctx, tx, s.opts.Control.opts.Scope.ID(), in.RequestID); err == nil {
					d, e := decodeExecution(existing.Document)
					if e != nil || existing.Domain != string(ControlExecution) || existing.ActorID != reqctx.GetUserID(ctx) || d.Operation != in {
						return controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
					}
					return nil
				} else if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				r, e := s.recoveryStatus(ctx)
				if e != nil {
					return e
				}
				if r.Epoch == "" || r.Epoch != in.ExpectedRecoveryEpoch {
					return controlError(409, "STORAGE_RECOVERY_EPOCH_CHANGED")
				}
				if gate != nil {
					d, e := decodeExecution(gate.Document)
					if e != nil {
						return e
					}
					if d.RecoveryEpoch == r.Epoch {
						return controlError(409, "STORAGE_RECOVERY_EPOCH_CHANGED")
					}
				}
				if gate == nil && in.ExpectedRevision != 0 || gate != nil && gate.Revision != in.ExpectedRevision {
					return controlError(409, "STORAGE_CONTROL_REVISION_CONFLICT")
				}
				rows, e := s.opts.Control.repo.ConfigurationsTx(ctx, tx, s.opts.Control.opts.Scope.ID())
				if e != nil {
					return e
				}
				present := map[ControlDomain]bool{}
				for _, row := range rows {
					present[ControlDomain(row.Domain)] = true
				}
				s.opts.Control.mu.Lock()
				domains := make([]ControlDomain, 0, len(s.opts.Control.bindings))
				for domain := range s.opts.Control.bindings {
					domains = append(domains, domain)
				}
				s.opts.Control.mu.Unlock()
				for _, domain := range domains {
					floor, e := s.recoveryFloor(ctx, domain)
					if e != nil {
						return e
					}
					if floor > 0 && !present[domain] {
						return controlError(409, "STORAGE_RECOVERY_RESOURCE_MISSING")
					}
				}
				for _, row := range rows {
					if e := s.validateRecoveryRow(ctx, tx, row); e != nil {
						return e
					}
					if ControlDomain(row.Domain) == ControlExecution {
						continue
					}
					floor, e := s.recoveryFloor(ctx, ControlDomain(row.Domain))
					if e != nil {
						return e
					}
					history, e := s.opts.Control.repo.MaximumRevisionTx(ctx, tx, row.ScopeID, row.Domain)
					if e != nil {
						return e
					}
					next := max(row.Revision, floor, history, recoveryEntryFloor(row.Document))
					if next == math.MaxInt64 {
						return ErrStateUnavailable
					}
					next++
					raw, e := s.prepareRecoveryDocument(ctx, tx, row, next)
					if e != nil {
						return e
					}
					var body map[string]json.RawMessage
					if json.Unmarshal(raw, &body) != nil {
						return ErrStateUnavailable
					}
					body["recovery"], _ = json.Marshal(RecoveryStamp{r.Epoch, in.RequestID, row.Revision, row.Digest, s.recoveryEnvironment()})
					raw, _ = json.Marshal(body)
					key := uuid.NewSHA1(uuid.MustParse(in.RequestID), []byte(row.Domain)).String()
					v, e := s.saveRecoveryVersion(ctx, tx, &row, ControlDomain(row.Domain), next, key, raw)
					if e != nil {
						return e
					}
					versions = append(versions, v)
				}
				floor, e := s.recoveryFloor(ctx, ControlExecution)
				if e != nil {
					return e
				}
				history, e := s.opts.Control.repo.MaximumRevisionTx(ctx, tx, s.opts.Control.opts.Scope.ID(), string(ControlExecution))
				if e != nil {
					return e
				}
				next := max(in.ExpectedRevision, floor, history)
				if next == math.MaxInt64 {
					return ErrStateUnavailable
				}
				next++
				d := executionDocument{Version: 1, Mode: "paused", ReviewPolicies: true, Operation: in, PreparedRecoveryEpoch: r.Epoch}
				raw, _ := json.Marshal(d)
				v, e := s.saveRecoveryVersion(ctx, tx, gate, ControlExecution, next, in.RequestID, raw)
				if e != nil {
					return e
				}
				versions = append(versions, v)
				return nil
			})
		})
	})
	if e != nil {
		return ExecutionMutation{}, e
	}
	// 只在事务提交后派发；丢失派发仍由正式 TaskBus 的持久恢复接续。
	for _, v := range versions {
		run, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		if s.opts.Control.opts.Dispatch(run, s.opts.Control.message(v)) != nil {
			select {
			case s.opts.Control.recoverWake <- struct{}{}:
			default:
			}
		}
		cancel()
	}
	if s.opts.Control.opts.Changed != nil {
		s.opts.Control.opts.Changed()
	}
	return s.ExecutionRequest(ctx, in.RequestID)
}

func (s *Service) recoveryResourcesTx(ctx context.Context, tx *gorm.DB, epoch string) ([]RecoveryResource, error) {
	rows, e := s.opts.Control.repo.ConfigurationsTx(ctx, tx, s.opts.Control.opts.Scope.ID())
	if e != nil {
		return nil, e
	}
	out := []RecoveryResource{}
	present := map[ControlDomain]bool{}
	for _, row := range rows {
		present[ControlDomain(row.Domain)] = true
		if ControlDomain(row.Domain) == ControlExecution {
			continue
		}
		kind, _ := s.executionPolicyLabel(row)
		switch ControlDomain(row.Domain) {
		case ControlArchiveTargets:
			kind = "targets"
		case ControlBudget:
			kind = "budget"
		case ControlTableMonitoring:
			kind = "monitoring"
		}
		item := RecoveryResource{Domain: row.Domain, Kind: kind, Revision: row.Revision, State: "pending"}
		stamp := recoveryStamp(row.Document)
		if stamp == nil || stamp.Epoch != epoch || stamp.Environment != s.recoveryEnvironment() {
			item.Problem = "STORAGE_RECOVERY_REBUILD_REQUIRED"
		} else if e = s.validateRecoveryRow(ctx, tx, row); e != nil {
			item.Problem = "STORAGE_RECOVERY_AUDIT_MISMATCH"
		} else if row.AppliedRevision != row.Revision || row.PublicationState != "applied" {
			item.Problem = "STORAGE_EXECUTION_PUBLICATION_PENDING"
		} else if _, e = s.prepareRecoveryDocument(ctx, tx, row, row.Revision); e != nil {
			item.Problem = "STORAGE_RECOVERY_SOURCE_CHANGED"
		} else if e = s.checkRecoveryProjection(ctx, row); e != nil {
			item.Problem = "STORAGE_EXECUTION_PUBLICATION_PENDING"
		} else {
			item.State = "ready"
		}
		if item.State == "ready" && ControlDomain(row.Domain) == ControlArchiveTargets {
			d, _ := decodeArchiveTargets(row.Document)
			for _, entry := range d.Entries {
				if s.recoveryArchives.CheckRecoveryTarget(ctx, effectiveArchiveTarget(entry, s.opts.Control.opts.Scope)) != nil {
					item.State = "pending"
					item.Problem = "STORAGE_RECOVERY_TARGET_REQUIRED"
					break
				}
			}
		}
		out = append(out, item)
	}
	legacy, e := s.recoveryLegacyResources(ctx, tx, present)
	if e != nil {
		return nil, e
	}
	out = append(out, legacy...)
	return out, nil
}

func (s *Service) checkRecoveryProjection(ctx context.Context, row m.ControlConfiguration) error {
	if ControlDomain(row.Domain) == ControlBudget {
		snap, e := s.opts.Budget.ControlSnapshot(ctx)
		if e != nil || snap.Projection == nil {
			return ErrStateUnavailable
		}
		p := snap.Projection
		if p.Digest != row.Digest || p.ActorID != row.ActorID {
			return ErrStateUnavailable
		}
		if _, e = verifiedControlDocument(p.Document, p.Digest); e != nil {
			return e
		}
	}
	if strings.HasPrefix(row.Domain, "file-lifecycle.") {
		d, e := decodeLifecycleDocument(row.Document)
		if e != nil {
			return e
		}
		_, p, e := s.opts.Lifecycle.PolicySnapshot(ctx, d.SourceID)
		if e != nil || p == nil || p.Digest != row.Digest || p.Policy.ConfigurationDigest != row.Digest || p.Policy.SourceID != d.SourceID || p.Policy.Fingerprint != d.Fingerprint || lifecycleSettings(p.Policy) != d.Settings {
			return ErrStateUnavailable
		}
	}
	if ControlDomain(row.Domain) == ControlSnapshotPolicies {
		b, e := s.recoverySnapshots.snapshots.ReadPolicyBinding()
		d, decodeErr := decodeSnapshotPolicies(row.Document)
		if e != nil || decodeErr != nil || b.OriginDigest != d.OriginDigest {
			return ErrStateUnavailable
		}
	}
	for _, name := range recoveryFiles(ControlDomain(row.Domain)) {
		raw, e := readArchiveControlProjection(ctx, s.opts.Budget, name)
		if e != nil {
			return e
		}
		var p struct {
			Revision int64           `json:"revision"`
			Digest   string          `json:"digest"`
			Document json.RawMessage `json:"document"`
		}
		if json.Unmarshal(raw, &p) != nil || p.Revision != row.Revision {
			return ErrStateUnavailable
		}
		if len(p.Document) > 0 {
			if p.Digest != row.Digest {
				return ErrStateUnavailable
			}
			if _, e = verifiedControlDocument(p.Document, p.Digest); e != nil {
				return e
			}
		}
	}
	floor, e := s.recoveryFloor(ctx, ControlDomain(row.Domain))
	if e != nil || floor != row.Revision {
		return ErrStateUnavailable
	}
	return nil
}

// 已登记但尚未接管的旧启用值也必须可见，空数据库库存不是完成交接的证明。
func (s *Service) recoveryLegacyResources(ctx context.Context, tx *gorm.DB, present map[ControlDomain]bool) ([]RecoveryResource, error) {
	out := []RecoveryResource{}
	add := func(domain ControlDomain, kind string) {
		out = append(out, RecoveryResource{Domain: string(domain), Kind: kind, State: "pending", Problem: "STORAGE_RECOVERY_LEGACY_REQUIRED"})
	}
	if !present[ControlDatabaseRetention] {
		d, _, e := s.legacyRetention(ctx)
		if e != nil {
			return nil, e
		}
		if d.Policy.Enabled {
			add(ControlDatabaseRetention, "invocations")
		}
	}
	for _, id := range []string{repo.ProbeRetentionID, repo.DiscoveryRetentionID} {
		domain := systemRetentionDomain(id)
		if !present[domain] {
			d, _, e := s.legacySystemRetention(ctx, id)
			if e != nil {
				return nil, e
			}
			if d.Policies[id].Enabled {
				kind := "probes"
				if id == repo.DiscoveryRetentionID {
					kind = "discovery"
				}
				add(domain, kind)
			}
		}
	}
	if s.opts.Lifecycle != nil {
		for _, source := range s.opts.Lifecycle.RegisteredSources() {
			domain := lifecycleDomain(source.ID)
			if !present[domain] {
				p, _, e := s.opts.Lifecycle.PolicySnapshot(ctx, source.ID)
				if e != nil {
					return nil, e
				}
				if p.Enabled {
					add(domain, "file_logs")
				}
			}
		}
	}
	if s.recoveryArchives != nil {
		if !present[ControlArchiveTargets] {
			targets, e := s.recoveryArchives.LegacyTargets(ctx)
			if e != nil {
				return nil, e
			}
			if len(targets) > 0 {
				add(ControlArchiveTargets, "targets")
			}
		}
		if !present[ControlArchivePolicies] {
			policies, e := s.recoveryArchives.LegacyPolicies(ctx)
			if e != nil {
				return nil, e
			}
			for _, p := range policies {
				if p.Enabled || p.RemoteCleanupEnabled {
					add(ControlArchivePolicies, "archives")
					break
				}
			}
		}
	}
	if c := s.recoverySnapshots; c != nil && !present[ControlSnapshotPolicies] {
		d, _, e := c.snapshots.LegacyPolicySnapshot(ctx)
		if e != nil {
			return nil, e
		}
		for _, p := range d.Policies {
			if p.Enabled {
				add(ControlSnapshotPolicies, "snapshots")
				break
			}
		}
	}
	if c := s.recoveryBackups; c != nil && !present[ControlBackupPolicies] {
		rows, e := c.rows(ctx, tx)
		if e != nil {
			return nil, e
		}
		for _, p := range rows {
			if p.Enabled || p.DrillEnabled {
				add(ControlBackupPolicies, "backups")
				break
			}
		}
	}
	return out, nil
}
