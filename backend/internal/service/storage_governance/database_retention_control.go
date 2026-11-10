package storagegov

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// DatabaseRetentionSettings 只允许固定调用审计适配器，不接收表名、SQL 或任意过滤条件。
type DatabaseRetentionSettings struct {
	SuccessDays       int   `json:"success_days"`
	FailureDays       int   `json:"failure_days"`
	PendingHours      int   `json:"pending_hours"`
	QuarantineDays    int   `json:"quarantine_days"`
	BatchLimit        int   `json:"batch_limit"`
	MaxBytes          int64 `json:"max_bytes"`
	AdoptLegacy       bool  `json:"adopt_legacy"`
	RejectNewWhenFull bool  `json:"reject_new_when_full"`
}

func retentionSettings(p DatabaseRetentionPolicy) DatabaseRetentionSettings {
	return DatabaseRetentionSettings{p.SuccessDays, p.FailureDays, p.PendingHours, p.QuarantineDays, p.BatchLimit, p.MaxBytes, p.AdoptLegacy, p.RejectNewWhenFull}
}
func (p DatabaseRetentionSettings) policy() DatabaseRetentionPolicy {
	return DatabaseRetentionPolicy{SuccessDays: p.SuccessDays, FailureDays: p.FailureDays, PendingHours: p.PendingHours, QuarantineDays: p.QuarantineDays, BatchLimit: p.BatchLimit, MaxBytes: p.MaxBytes, AdoptLegacy: p.AdoptLegacy, RejectNewWhenFull: p.RejectNewWhenFull}
}

type DatabaseRetentionAdoption struct {
	RequestID    string `json:"request_id"`
	LegacyDigest string `json:"legacy_digest"`
	Confirm      bool   `json:"confirm"`
}
type DatabaseRetentionChange struct {
	RequestID        string                     `json:"request_id"`
	ExpectedRevision int64                      `json:"expected_revision"`
	Action           string                     `json:"action"`
	Settings         *DatabaseRetentionSettings `json:"settings,omitempty"`
	Enabled          *bool                      `json:"enabled,omitempty"`
	Confirm          bool                       `json:"confirm"`
}
type retentionOperation struct {
	Action   string                     `json:"action"`
	Adoption *DatabaseRetentionAdoption `json:"adoption,omitempty"`
	Change   *DatabaseRetentionChange   `json:"change,omitempty"`
}
type retentionControlDocument struct {
	Recovery     *RecoveryStamp            `json:"recovery,omitempty"`
	Version      int                       `json:"version"`
	OriginDigest string                    `json:"origin_digest"`
	Source       string                    `json:"source"`
	Settings     DatabaseRetentionSettings `json:"settings"`
	Enabled      bool                      `json:"enabled"`
	Operation    retentionOperation        `json:"operation"`
}
type DatabaseRetentionConfiguration struct {
	EffectiveCapacity *DatabaseRetentionSettings `json:"effective_capacity"`
	Authority         string                     `json:"authority"`
	State             string                     `json:"state"`
	Ready             bool                       `json:"ready"`
	ExecutionReady    bool                       `json:"execution_ready"`
	DesiredRevision   int64                      `json:"desired_revision"`
	AppliedRevision   int64                      `json:"applied_revision"`
	SnapshotRevision  *int64                     `json:"snapshot_revision"`
	LegacyDigest      string                     `json:"legacy_digest"`
	Problem           string                     `json:"problem,omitempty"`
}
type DatabaseRetentionMutation struct {
	RequestID         string                         `json:"request_id"`
	OperationRevision int64                          `json:"operation_revision"`
	Configuration     DatabaseRetentionConfiguration `json:"configuration"`
}
type retentionBinding struct {
	Version      int    `json:"version"`
	Scope        string `json:"scope"`
	Revision     int64  `json:"revision"`
	OriginDigest string `json:"origin_digest"`
}

func decodeRetentionControl(raw []byte) (retentionControlDocument, error) {
	var d retentionControlDocument
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&d) != nil || dec.Decode(new(any)) != io.EOF || d.Version != 1 || len(d.OriginDigest) != 64 || len(d.Source) != 64 || !validDatabasePolicy(d.Settings.policy()) || (d.Operation.Action != "adopt" || d.Operation.Adoption == nil || d.Operation.Change != nil) && (d.Operation.Action != "change" || d.Operation.Change == nil || d.Operation.Adoption != nil) {
		return d, controlError(400, "DATABASE_RETENTION_DOCUMENT_INVALID")
	}
	return d, nil
}

// 连接标识仅保存摘要；凭据轮换或目标库变化后需重新保存并确认，不能沿用旧清理许可。
func (s *Service) retentionSource() string {
	d, ok := s.opts.DB.Dialector.(*postgres.Dialector)
	if !ok || d.Config == nil || d.Config.DSN == "" {
		return ""
	}
	return controlDigest([]byte(s.opts.Control.opts.Scope.ID() + "\x00" + d.Config.DSN + "\x00" + capmodel.InvocationTrace{}.TableName() + "\x00" + capmodel.CapabilityEventPublication{}.TableName()))
}
func (s *Service) legacyRetention(ctx context.Context) (databaseRetentionDocument, string, error) {
	raw, e := readArchiveControlProjection(ctx, s.opts.Budget, "database-retention")
	if e != nil {
		return databaseRetentionDocument{}, "", e
	}
	d, e := s.databaseDocument(raw)
	if e != nil || !validDatabasePolicy(d.Policy) || len(d.Runs) > 1000 {
		return d, "", ErrStateUnavailable
	}
	b, _ := json.Marshal(d)
	return d, controlDigest(b), nil
}
func (s *Service) bindDatabaseRetentionControl() error {
	if s.opts.Budget == nil || s.opts.DB == nil {
		return nil
	}
	if e := s.opts.Control.Bind(ControlDatabaseRetention, ControlBinding{Validate: func(raw json.RawMessage) error { _, e := decodeRetentionControl(raw); return e }, Apply: s.publishRetention}); e != nil {
		return e
	}
	_, e := s.opts.Budget.Document(context.Background(), "database-retention-binding", func(raw []byte) ([]byte, error) {
		if len(raw) > 0 {
			var b retentionBinding
			if json.Unmarshal(raw, &b) != nil || b.Version != 1 || b.Scope != s.opts.Control.opts.Scope.ID() || b.Revision < 0 {
				return nil, ErrStateUnavailable
			}
			return nil, nil
		}
		return json.Marshal(retentionBinding{Version: 1, Scope: s.opts.Control.opts.Scope.ID()})
	})
	return e
}
func (s *Service) publishRetention(ctx context.Context, p ControlPublication) error {
	if p.Scope != s.opts.Control.opts.Scope {
		return ErrStateUnavailable
	}
	d, e := decodeRetentionControl(p.Document)
	if e != nil {
		return e
	}
	_, hash, legacyError := s.legacyRetention(ctx)
	_, e = updateArchiveControlProjection(ctx, s.opts.Budget, "database-retention-binding", func(raw []byte) ([]byte, error) {
		var b retentionBinding
		if json.Unmarshal(raw, &b) != nil || b.Version != 1 || b.Scope != p.Scope.ID() || b.Revision > p.Revision || b.Revision > 0 && b.OriginDigest != d.OriginDigest {
			return nil, ErrStateUnavailable
		}
		if b.Revision == 0 {
			if legacyError != nil || hash != d.OriginDigest {
				return nil, controlError(409, "DATABASE_RETENTION_LEGACY_CHANGED")
			}
		}
		return json.Marshal(retentionBinding{1, p.Scope.ID(), p.Revision, d.OriginDigest})
	})
	if e != nil {
		return e
	}
	_, e = updateArchiveControlProjection(ctx, s.opts.Budget, "database-retention-control", func(raw []byte) ([]byte, error) {
		if len(raw) > 0 {
			var old snapshotPolicyProjection
			if json.Unmarshal(raw, &old) != nil || old.Version != 1 || old.Scope != p.Scope || old.Revision > p.Revision || old.Revision == p.Revision && old.Digest != p.Digest {
				return nil, ErrStateUnavailable
			}
		}
		return json.Marshal(snapshotPolicyProjection{Version: 1, Scope: p.Scope, Revision: p.Revision, Digest: p.Digest, Document: p.Document, ActorID: p.ActorID, UpdatedAt: p.UpdatedAt})
	})
	return e
}

// retentionControlTx 仅在配置锁内读取权威与本机快照；调用方复用同一事务执行清理。
func (s *Service) retentionControlTx(ctx context.Context, tx *gorm.DB, row *m.ControlConfiguration) (DatabaseRetentionPolicy, DatabaseRetentionConfiguration, error) {
	p := DatabaseRetentionPolicy{}
	c := DatabaseRetentionConfiguration{Authority: "unavailable", State: "unconfirmed"}
	fail := func(code string) (DatabaseRetentionPolicy, DatabaseRetentionConfiguration, error) {
		c.Problem = code
		c.Authority = "unavailable"
		c.State = "unconfirmed"
		c.Ready = false
		c.ExecutionReady = false
		c.EffectiveCapacity = nil
		return p, c, controlError(503, code)
	}
	var b retentionBinding
	raw, e := readArchiveControlProjection(ctx, s.opts.Budget, "database-retention-binding")
	if e != nil || json.Unmarshal(raw, &b) != nil || b.Version != 1 || b.Scope != s.opts.Control.opts.Scope.ID() {
		return fail("DATABASE_RETENTION_BINDING_UNAVAILABLE")
	}
	raw, e = readArchiveControlProjection(ctx, s.opts.Budget, "database-retention-control")
	if e != nil {
		return fail("DATABASE_RETENTION_SNAPSHOT_UNAVAILABLE")
	}
	c.Ready = s.opts.ControlWorkerEnabled && !s.opts.Control.ConsumerExited()
	legacy, hash, e := s.legacyRetention(ctx)
	if e != nil {
		return fail("DATABASE_RETENTION_LEGACY_UNAVAILABLE")
	}
	if row == nil {
		if b.Revision > 0 || len(raw) > 0 {
			return fail("DATABASE_RETENTION_RESTORE_CONFIRMATION_REQUIRED")
		}
		c.Authority = "legacy"
		c.State = "legacy"
		c.LegacyDigest = hash
		p = legacy.Policy
		p.Enabled = false
		effective := retentionSettings(legacy.Policy)
		c.EffectiveCapacity = &effective
		return p, c, nil
	}
	if row.HostID != s.opts.Control.opts.Scope.HostID || row.FilesystemID != s.opts.Control.opts.Scope.FilesystemID || row.InstanceID != s.opts.Control.opts.Scope.InstanceID {
		return fail("STORAGE_CONTROL_SCOPE_MISMATCH")
	}
	if _, e = verifiedControlDocument(row.Document, row.Digest); e != nil {
		return fail("DATABASE_RETENTION_DOCUMENT_INVALID")
	}
	d, e := decodeRetentionControl(row.Document)
	if e != nil {
		return fail("DATABASE_RETENTION_DOCUMENT_INVALID")
	}
	if b.Revision > row.Revision || b.Revision > 0 && b.OriginDigest != d.OriginDigest || hash != d.OriginDigest {
		return fail("DATABASE_RETENTION_RESTORE_CONFIRMATION_REQUIRED")
	}
	c.Authority = "database"
	c.State = row.PublicationState
	c.DesiredRevision = row.Revision
	c.AppliedRevision = row.AppliedRevision
	c.LegacyDigest = d.OriginDigest
	c.Problem = row.Problem
	p = d.Settings.policy()
	p.Enabled = d.Enabled
	p.Revision = row.Revision
	p.OwnerInstance = row.InstanceID
	p.Actor = row.ActorID
	p.UpdatedAt = row.UpdatedAt
	effective := d.Settings
	c.EffectiveCapacity = &effective
	if len(raw) == 0 && row.AppliedRevision == 0 {
		return p, c, nil
	}
	var projection snapshotPolicyProjection
	if json.Unmarshal(raw, &projection) != nil || projection.Version != 1 || projection.Scope != s.opts.Control.opts.Scope || projection.Revision != row.AppliedRevision || projection.Revision < 1 || b.Revision != projection.Revision {
		return fail("DATABASE_RETENTION_SNAPSHOT_UNCONFIRMED")
	}
	v, lookupError := s.opts.Control.repo.RevisionTx(ctx, tx, b.Scope, string(ControlDatabaseRetention), projection.Revision)
	if e = lookupError; e != nil || v.Digest != projection.Digest || v.ActorID != projection.ActorID || !v.CreatedAt.Equal(projection.UpdatedAt) {
		return fail("DATABASE_RETENTION_SNAPSHOT_UNCONFIRMED")
	}
	if _, e = verifiedControlDocument(v.Document, v.Digest); e != nil {
		return fail("DATABASE_RETENTION_SNAPSHOT_UNCONFIRMED")
	}
	if _, e = verifiedControlDocument(projection.Document, projection.Digest); e != nil {
		return fail("DATABASE_RETENTION_SNAPSHOT_UNCONFIRMED")
	}
	active, e := decodeRetentionControl(projection.Document)
	if e != nil {
		return fail("DATABASE_RETENTION_SNAPSHOT_UNCONFIRMED")
	}
	c.SnapshotRevision = &projection.Revision
	c.ExecutionReady = row.PublicationState == "applied" && row.Revision == row.AppliedRevision && d.Source == s.retentionSource()
	if d.Source != s.retentionSource() {
		c.Problem = "DATABASE_RETENTION_SOURCE_CHANGED"
		c.State = "unconfirmed"
	}
	// 待发布时容量限制只能收紧；停用或扩大限额须等执行确认，清理一律暂停。
	effective.RejectNewWhenFull = effective.RejectNewWhenFull || active.Settings.RejectNewWhenFull
	if active.Settings.RejectNewWhenFull && (!d.Settings.RejectNewWhenFull || active.Settings.MaxBytes < effective.MaxBytes) {
		effective.MaxBytes = active.Settings.MaxBytes
	}
	return p, c, nil
}
func (s *Service) loadRetentionControl(ctx context.Context) (DatabaseRetentionPolicy, DatabaseRetentionConfiguration, error) {
	c := DatabaseRetentionConfiguration{Authority: "unavailable", State: "unavailable", Problem: "STORAGE_CONTROL_SCHEMA_NOT_READY"}
	p := DatabaseRetentionPolicy{}
	if s.opts.Control == nil || s.opts.Budget == nil || s.opts.DB == nil {
		c.Problem = "STORAGE_GOVERNANCE_UNAVAILABLE"
		return p, c, controlError(503, c.Problem)
	}
	if err := s.opts.Control.CheckReady(ctx); err != nil {
		c.Problem = dto.CodeOf(err)
		return p, c, err
	}
	e := s.opts.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.opts.Control.repo.WithCurrentTx(ctx, tx, s.opts.Control.opts.Scope.ID(), string(ControlDatabaseRetention), func(row *m.ControlConfiguration) error {
			var e error
			p, c, e = s.retentionControlTx(ctx, tx, row)
			return e
		})
	})
	if e != nil && dto.CodeOf(e) == "" {
		e = admissionUnavailable(e)
		c.Problem = dto.CodeOf(e)
	}
	return p, c, e
}
func (s *Service) retentionWritable(ctx context.Context) (DatabaseRetentionPolicy, DatabaseRetentionConfiguration, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return DatabaseRetentionPolicy{}, DatabaseRetentionConfiguration{}, ErrRootRequired
	}
	p, c, e := s.loadRetentionControl(ctx)
	if e == nil && !c.Ready {
		e = controlError(503, "STORAGE_CONTROL_WORKER_UNAVAILABLE")
	}
	return p, c, e
}
func (s *Service) AdoptDatabaseRetention(ctx context.Context, in DatabaseRetentionAdoption) (DatabaseRetentionMutation, error) {
	if !in.Confirm || !validTargetUUID(in.RequestID) {
		return DatabaseRetentionMutation{}, ErrPolicyInvalid
	}
	op := retentionOperation{Action: "adopt", Adoption: &in}
	if out, ok, e := s.retentionReplay(ctx, in.RequestID, op); ok {
		return out, e
	}
	p, c, e := s.retentionWritable(ctx)
	if e == nil && s.retentionSource() == "" {
		e = controlError(503, "DATABASE_RETENTION_SOURCE_UNAVAILABLE")
	}
	if e != nil {
		return DatabaseRetentionMutation{}, e
	}
	if c.Authority != "legacy" || in.LegacyDigest != c.LegacyDigest {
		return DatabaseRetentionMutation{}, controlError(409, "DATABASE_RETENTION_LEGACY_CHANGED")
	}
	legacy, hash, e := s.legacyRetention(ctx)
	if e != nil || hash != in.LegacyDigest {
		return DatabaseRetentionMutation{}, ErrStateUnavailable
	}
	for _, r := range legacy.Runs {
		if r.State == "unconfirmed" {
			return DatabaseRetentionMutation{}, controlError(409, "DATABASE_RETENTION_LEGACY_RESULT_UNCONFIRMED")
		}
	}
	d := retentionControlDocument{nil, 1, hash, s.retentionSource(), retentionSettings(p), false, op}
	return s.saveRetentionControl(ctx, in.RequestID, 0, d)
}
func (s *Service) ChangeDatabaseRetention(ctx context.Context, in DatabaseRetentionChange) (DatabaseRetentionMutation, error) {
	if !in.Confirm || !validTargetUUID(in.RequestID) || in.ExpectedRevision < 1 || in.Action != "save" && in.Action != "status" || in.Action == "save" && (in.Settings == nil || in.Enabled != nil || !validDatabasePolicy(in.Settings.policy())) || in.Action == "status" && (in.Enabled == nil || in.Settings != nil) {
		return DatabaseRetentionMutation{}, ErrPolicyInvalid
	}
	op := retentionOperation{Action: "change", Change: &in}
	if out, ok, e := s.retentionReplay(ctx, in.RequestID, op); ok {
		return out, e
	}
	_, c, e := s.retentionWritable(ctx)
	if e != nil {
		return DatabaseRetentionMutation{}, e
	}
	if c.Authority != "database" {
		return DatabaseRetentionMutation{}, controlError(428, "DATABASE_RETENTION_ADOPTION_REQUIRED")
	}
	if c.DesiredRevision != in.ExpectedRevision {
		return DatabaseRetentionMutation{}, controlError(409, "STORAGE_CONTROL_REVISION_CONFLICT")
	}
	state, e := s.opts.Control.read(ctx, ControlDatabaseRetention)
	if e != nil {
		return DatabaseRetentionMutation{}, e
	}
	if state.DesiredRevision != in.ExpectedRevision {
		return DatabaseRetentionMutation{}, controlError(409, "STORAGE_CONTROL_REVISION_CONFLICT")
	}
	d, e := decodeRetentionControl(state.Document)
	if e != nil {
		return DatabaseRetentionMutation{}, e
	}
	if in.Action == "save" {
		d.Settings = *in.Settings
		d.Enabled = false
		d.Source = s.retentionSource()
	} else {
		if *in.Enabled && (!s.opts.DatabaseRetentionAllowed || !s.opts.LifecycleWorkerEnabled || d.Source != s.retentionSource()) {
			return DatabaseRetentionMutation{}, ErrStateUnavailable
		}
		if *in.Enabled {
			if !s.retentionLedger().Ready(ctx) {
				return DatabaseRetentionMutation{}, controlError(503, "DATABASE_RETENTION_LEDGER_UNAVAILABLE")
			}
			if e = s.dbRetention.Ready(ctx); e != nil {
				return DatabaseRetentionMutation{}, e
			}
		}
		d.Enabled = *in.Enabled
	}
	if d.Settings.RejectNewWhenFull && !s.opts.DatabaseRetentionAllowed {
		return DatabaseRetentionMutation{}, ErrStateUnavailable
	}
	d.Operation = op
	return s.saveRetentionControl(ctx, in.RequestID, in.ExpectedRevision, d)
}
func (s *Service) saveRetentionControl(ctx context.Context, key string, rev int64, d retentionControlDocument) (DatabaseRetentionMutation, error) {
	raw, e := json.Marshal(d)
	if e != nil {
		return DatabaseRetentionMutation{}, e
	}
	_, e = s.opts.Control.Save(ctx, ControlInput{key, rev, ControlDatabaseRetention, raw})
	if e != nil {
		return DatabaseRetentionMutation{}, e
	}
	return s.DatabaseRetentionRequest(ctx, key)
}
func (s *Service) retentionReplay(ctx context.Context, key string, op retentionOperation) (DatabaseRetentionMutation, bool, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return DatabaseRetentionMutation{}, true, ErrRootRequired
	}
	if s.opts.Control == nil {
		return DatabaseRetentionMutation{}, true, ErrStateUnavailable
	}
	v, e := s.opts.Control.repo.Request(ctx, s.opts.Control.opts.Scope.ID(), key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return DatabaseRetentionMutation{}, false, nil
	}
	if e != nil {
		return DatabaseRetentionMutation{}, true, ErrStateUnavailable
	}
	d, e := decodeRetentionControl(v.Document)
	a, _ := json.Marshal(op)
	b, _ := json.Marshal(d.Operation)
	if e != nil || v.Domain != string(ControlDatabaseRetention) || v.ActorID != reqctx.GetUserID(ctx) || !bytes.Equal(a, b) {
		return DatabaseRetentionMutation{}, true, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
	}
	out, e := s.DatabaseRetentionRequest(ctx, key)
	return out, true, e
}
func (s *Service) DatabaseRetentionRequest(ctx context.Context, key string) (DatabaseRetentionMutation, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return DatabaseRetentionMutation{}, ErrRootRequired
	}
	if s.opts.Control == nil || !validTargetUUID(key) {
		return DatabaseRetentionMutation{}, ErrPolicyInvalid
	}
	v, e := s.opts.Control.repo.Request(ctx, s.opts.Control.opts.Scope.ID(), key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return DatabaseRetentionMutation{}, controlError(404, "STORAGE_CONTROL_REQUEST_NOT_FOUND")
	}
	if e != nil {
		return DatabaseRetentionMutation{}, ErrStateUnavailable
	}
	if v.Domain != string(ControlDatabaseRetention) || v.ActorID != reqctx.GetUserID(ctx) {
		return DatabaseRetentionMutation{}, controlError(403, "STORAGE_CONTROL_REQUEST_FORBIDDEN")
	}
	if _, e = verifiedControlDocument(v.Document, v.Digest); e != nil {
		return DatabaseRetentionMutation{}, e
	}
	_, c, _ := s.loadRetentionControl(ctx)
	return DatabaseRetentionMutation{key, v.Revision, c}, nil
}
