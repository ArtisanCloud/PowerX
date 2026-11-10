package storagegov

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	capm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"io"
)

// SystemTableRetentionSettings 不接受表名、时间字段、SQL 或可扩展删除谓词。
type SystemTableRetentionSettings struct {
	RetentionDays int `json:"retention_days"`
	BatchLimit    int `json:"batch_limit"`
}

func (p SystemTableRetentionSettings) policy() SystemTableRetentionPolicy {
	return SystemTableRetentionPolicy{RetentionDays: p.RetentionDays, BatchLimit: p.BatchLimit}
}
func systemRetentionSettings(p SystemTableRetentionPolicy) SystemTableRetentionSettings {
	return SystemTableRetentionSettings{p.RetentionDays, p.BatchLimit}
}

type SystemTableRetentionAdoption struct {
	RequestID    string `json:"request_id"`
	LegacyDigest string `json:"legacy_digest"`
	Confirm      bool   `json:"confirm"`
}
type SystemTableRetentionChange struct {
	RequestID        string                        `json:"request_id"`
	ExpectedRevision int64                         `json:"expected_revision"`
	Action           string                        `json:"action"`
	Settings         *SystemTableRetentionSettings `json:"settings,omitempty"`
	Enabled          *bool                         `json:"enabled,omitempty"`
	Confirm          bool                          `json:"confirm"`
}
type systemRetentionOperation struct {
	Action   string                        `json:"action"`
	Adoption *SystemTableRetentionAdoption `json:"adoption,omitempty"`
	Change   *SystemTableRetentionChange   `json:"change,omitempty"`
}
type systemRetentionControlDocument struct {
	Recovery     *RecoveryStamp               `json:"recovery,omitempty"`
	Version      int                          `json:"version"`
	TableID      string                       `json:"table_id"`
	OriginDigest string                       `json:"origin_digest"`
	Source       string                       `json:"source"`
	Settings     SystemTableRetentionSettings `json:"settings"`
	Enabled      bool                         `json:"enabled"`
	Operation    systemRetentionOperation     `json:"operation"`
}
type SystemTableRetentionConfiguration struct {
	Authority        string `json:"authority"`
	State            string `json:"state"`
	Ready            bool   `json:"ready"`
	ExecutionReady   bool   `json:"execution_ready"`
	DesiredRevision  int64  `json:"desired_revision"`
	AppliedRevision  int64  `json:"applied_revision"`
	SnapshotRevision *int64 `json:"snapshot_revision"`
	LegacyDigest     string `json:"legacy_digest"`
	Problem          string `json:"problem,omitempty"`
}
type SystemTableRetentionMutation struct {
	TableID           string                            `json:"table_id"`
	RequestID         string                            `json:"request_id"`
	OperationRevision int64                             `json:"operation_revision"`
	Configuration     SystemTableRetentionConfiguration `json:"configuration"`
}

func systemRetentionDomain(id string) ControlDomain {
	return ControlDomain("system-table-retention." + controlDigest([]byte(id))[:32])
}
func systemRetentionFiles(id string) (string, string) {
	name := "system-retention-probe"
	if id == repo.DiscoveryRetentionID {
		name = "system-retention-discovery"
	}
	return name + "-binding", name + "-control"
}
func decodeSystemRetentionControl(raw []byte, id string) (systemRetentionControlDocument, error) {
	var d systemRetentionControlDocument
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&d) != nil || decoder.Decode(new(any)) != io.EOF || d.Version != 1 || !repo.SystemRetentionSupported(id) || d.TableID != id || len(d.OriginDigest) != 64 || len(d.Source) != 64 || !validSystemTablePolicy(d.Settings.policy()) || (d.Operation.Action != "adopt" || d.Operation.Adoption == nil || d.Operation.Change != nil) && (d.Operation.Action != "change" || d.Operation.Change == nil || d.Operation.Adoption != nil) {
		return d, controlError(400, "SYSTEM_RETENTION_DOCUMENT_INVALID")
	}
	return d, nil
}
func (s *Service) systemRetentionSource(id string) string {
	d, ok := s.opts.DB.Dialector.(*postgres.Dialector)
	if !ok || d.Config == nil || d.Config.DSN == "" {
		return ""
	}
	relation := capm.HealthProbeResult{}.TableName()
	if id == repo.DiscoveryRetentionID {
		relation = capm.DiscoveryCacheEntry{}.TableName()
	}
	return controlDigest([]byte(s.opts.Control.opts.Scope.ID() + "\x00" + d.Config.DSN + "\x00" + id + "\x00" + relation))
}

// legacySystemRetention 只计算当前适配器的来源证明；另一适配器变化不作废本表策略。
func (s *Service) legacySystemRetention(ctx context.Context, id string) (systemTableRetentionDocument, string, error) {
	raw, e := readArchiveControlProjection(ctx, s.opts.Budget, "system-table-retention")
	if e != nil {
		return systemTableRetentionDocument{}, "", e
	}
	d, e := s.systemTableDocument(raw)
	if e != nil {
		return d, "", e
	}
	runs := []SystemTableRetentionRun{}
	for _, r := range d.Runs {
		if r.TableID == id {
			runs = append(runs, r)
		}
	}
	body, _ := json.Marshal(struct {
		Owner   string                     `json:"owner"`
		Policy  SystemTableRetentionPolicy `json:"policy"`
		Runs    []SystemTableRetentionRun  `json:"runs"`
		Preview *SystemTableRetentionRun   `json:"preview,omitempty"`
	}{d.OwnerInstance, d.Policies[id], runs, func() *SystemTableRetentionRun {
		if p, ok := d.Previews[id]; ok {
			return &p
		}
		return nil
	}()})
	return d, controlDigest(body), nil
}
func (s *Service) bindSystemTableRetentionControl() error {
	if s.opts.DB == nil || s.opts.Budget == nil {
		return nil
	}
	for _, id := range []string{repo.ProbeRetentionID, repo.DiscoveryRetentionID} {
		id := id
		if e := s.opts.Control.Bind(systemRetentionDomain(id), ControlBinding{Validate: func(raw json.RawMessage) error { _, e := decodeSystemRetentionControl(raw, id); return e }, Apply: func(ctx context.Context, p ControlPublication) error { return s.publishSystemRetention(ctx, id, p) }}); e != nil {
			return e
		}
		binding, _ := systemRetentionFiles(id)
		_, e := s.opts.Budget.Document(context.Background(), binding, func(raw []byte) ([]byte, error) {
			if len(raw) > 0 {
				var b retentionBinding
				if json.Unmarshal(raw, &b) != nil || b.Version != 1 || b.Scope != s.opts.Control.opts.Scope.ID() || b.Revision < 0 {
					return nil, ErrStateUnavailable
				}
				return nil, nil
			}
			return json.Marshal(retentionBinding{Version: 1, Scope: s.opts.Control.opts.Scope.ID()})
		})
		if e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) publishSystemRetention(ctx context.Context, id string, p ControlPublication) error {
	if p.Scope != s.opts.Control.opts.Scope || p.Domain != systemRetentionDomain(id) {
		return ErrStateUnavailable
	}
	d, e := decodeSystemRetentionControl(p.Document, id)
	if e != nil {
		return e
	}
	_, hash, legacyError := s.legacySystemRetention(ctx, id)
	binding, projection := systemRetentionFiles(id)
	_, e = updateArchiveControlProjection(ctx, s.opts.Budget, binding, func(raw []byte) ([]byte, error) {
		var b retentionBinding
		if json.Unmarshal(raw, &b) != nil || b.Version != 1 || b.Scope != p.Scope.ID() || b.Revision > p.Revision || b.Revision > 0 && b.OriginDigest != d.OriginDigest {
			return nil, ErrStateUnavailable
		}
		if b.Revision == 0 && (legacyError != nil || hash != d.OriginDigest) {
			return nil, controlError(409, "SYSTEM_RETENTION_LEGACY_CHANGED")
		}
		return json.Marshal(retentionBinding{1, p.Scope.ID(), p.Revision, d.OriginDigest})
	})
	if e != nil {
		return e
	}
	_, e = updateArchiveControlProjection(ctx, s.opts.Budget, projection, func(raw []byte) ([]byte, error) {
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
func (s *Service) systemRetentionControlTx(ctx context.Context, tx *gorm.DB, id string, row *m.ControlConfiguration) (SystemTableRetentionPolicy, SystemTableRetentionConfiguration, error) {
	p := SystemTableRetentionPolicy{}
	c := SystemTableRetentionConfiguration{Authority: "unavailable", State: "unconfirmed"}
	fail := func(code string) (SystemTableRetentionPolicy, SystemTableRetentionConfiguration, error) {
		c.Authority = "unavailable"
		c.State = "unconfirmed"
		c.Ready = false
		c.ExecutionReady = false
		c.Problem = code
		return p, c, controlError(503, code)
	}
	binding, projection := systemRetentionFiles(id)
	raw, e := readArchiveControlProjection(ctx, s.opts.Budget, binding)
	var b retentionBinding
	if e != nil || json.Unmarshal(raw, &b) != nil || b.Version != 1 || b.Scope != s.opts.Control.opts.Scope.ID() {
		return fail("SYSTEM_RETENTION_BINDING_UNAVAILABLE")
	}
	raw, e = readArchiveControlProjection(ctx, s.opts.Budget, projection)
	if e != nil {
		return fail("SYSTEM_RETENTION_SNAPSHOT_UNAVAILABLE")
	}
	legacy, hash, e := s.legacySystemRetention(ctx, id)
	if e != nil {
		return fail("SYSTEM_RETENTION_LEGACY_UNAVAILABLE")
	}
	c.Ready = s.opts.ControlWorkerEnabled && !s.opts.Control.ConsumerExited()
	if row == nil {
		if b.Revision > 0 || len(raw) > 0 {
			return fail("SYSTEM_RETENTION_RESTORE_CONFIRMATION_REQUIRED")
		}
		c.Authority = "legacy"
		c.State = "legacy"
		c.LegacyDigest = hash
		p = legacy.Policies[id]
		p.Enabled = false
		return p, c, nil
	}
	if row.Domain != string(systemRetentionDomain(id)) || row.HostID != s.opts.Control.opts.Scope.HostID || row.FilesystemID != s.opts.Control.opts.Scope.FilesystemID || row.InstanceID != s.opts.Control.opts.Scope.InstanceID {
		return fail("STORAGE_CONTROL_SCOPE_MISMATCH")
	}
	if _, e = verifiedControlDocument(row.Document, row.Digest); e != nil {
		return fail("SYSTEM_RETENTION_DOCUMENT_INVALID")
	}
	d, e := decodeSystemRetentionControl(row.Document, id)
	if e != nil {
		return fail("SYSTEM_RETENTION_DOCUMENT_INVALID")
	}
	if b.Revision > row.Revision || b.Revision > 0 && b.OriginDigest != d.OriginDigest || hash != d.OriginDigest {
		return fail("SYSTEM_RETENTION_RESTORE_CONFIRMATION_REQUIRED")
	}
	c.Authority = "database"
	c.State = row.PublicationState
	c.DesiredRevision = row.Revision
	c.AppliedRevision = row.AppliedRevision
	c.LegacyDigest = d.OriginDigest
	c.Problem = row.Problem
	p = d.Settings.policy()
	p.Enabled = d.Enabled
	p.Actor = row.ActorID
	p.UpdatedAt = row.UpdatedAt
	if len(raw) == 0 && row.AppliedRevision == 0 {
		return p, c, nil
	}
	var snap snapshotPolicyProjection
	if json.Unmarshal(raw, &snap) != nil || snap.Version != 1 || snap.Scope != s.opts.Control.opts.Scope || snap.Revision != row.AppliedRevision || snap.Revision < 1 || b.Revision != snap.Revision {
		return fail("SYSTEM_RETENTION_SNAPSHOT_UNCONFIRMED")
	}
	v, e := s.opts.Control.repo.RevisionTx(ctx, tx, b.Scope, string(systemRetentionDomain(id)), snap.Revision)
	if e != nil || v.Digest != snap.Digest || v.ActorID != snap.ActorID || !v.CreatedAt.Equal(snap.UpdatedAt) {
		return fail("SYSTEM_RETENTION_SNAPSHOT_UNCONFIRMED")
	}
	if _, e = verifiedControlDocument(v.Document, v.Digest); e != nil {
		return fail("SYSTEM_RETENTION_SNAPSHOT_UNCONFIRMED")
	}
	if _, e = verifiedControlDocument(snap.Document, snap.Digest); e != nil {
		return fail("SYSTEM_RETENTION_SNAPSHOT_UNCONFIRMED")
	}
	if _, e = decodeSystemRetentionControl(snap.Document, id); e != nil {
		return fail("SYSTEM_RETENTION_SNAPSHOT_UNCONFIRMED")
	}
	c.SnapshotRevision = &snap.Revision
	c.ExecutionReady = row.PublicationState == "applied" && row.Revision == row.AppliedRevision && d.Source == s.systemRetentionSource(id)
	if d.Source != s.systemRetentionSource(id) {
		c.State = "unconfirmed"
		c.Problem = "SYSTEM_RETENTION_SOURCE_CHANGED"
	}
	return p, c, nil
}
func (s *Service) loadSystemRetentionControl(ctx context.Context, id string) (SystemTableRetentionPolicy, SystemTableRetentionConfiguration, error) {
	p := SystemTableRetentionPolicy{}
	c := SystemTableRetentionConfiguration{Authority: "unavailable", State: "unavailable", Problem: "STORAGE_CONTROL_SCHEMA_NOT_READY"}
	if !repo.SystemRetentionSupported(id) {
		return p, c, ErrSystemTableNotFound
	}
	if s.opts.Control == nil || s.opts.Budget == nil || s.opts.DB == nil || !s.opts.Control.Ready(ctx) {
		return p, c, ErrStateUnavailable
	}
	e := s.opts.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.opts.Control.repo.WithCurrentTx(ctx, tx, s.opts.Control.opts.Scope.ID(), string(systemRetentionDomain(id)), func(row *m.ControlConfiguration) error {
			var e error
			p, c, e = s.systemRetentionControlTx(ctx, tx, id, row)
			return e
		})
	})
	return p, c, e
}
func (s *Service) systemRetentionWritable(ctx context.Context, id string) (SystemTableRetentionPolicy, SystemTableRetentionConfiguration, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return SystemTableRetentionPolicy{}, SystemTableRetentionConfiguration{}, ErrRootRequired
	}
	p, c, e := s.loadSystemRetentionControl(ctx, id)
	if e == nil && !c.Ready {
		e = controlError(503, "STORAGE_CONTROL_WORKER_UNAVAILABLE")
	}
	return p, c, e
}
func (s *Service) AdoptSystemTableRetention(ctx context.Context, id string, in SystemTableRetentionAdoption) (SystemTableRetentionMutation, error) {
	if !in.Confirm || !validTargetUUID(in.RequestID) {
		return SystemTableRetentionMutation{}, ErrPolicyInvalid
	}
	op := systemRetentionOperation{Action: "adopt", Adoption: &in}
	if out, ok, e := s.systemRetentionReplay(ctx, id, in.RequestID, op); ok {
		return out, e
	}
	p, c, e := s.systemRetentionWritable(ctx, id)
	if e != nil {
		return SystemTableRetentionMutation{}, e
	}
	if c.Authority != "legacy" || c.LegacyDigest != in.LegacyDigest {
		return SystemTableRetentionMutation{}, controlError(409, "SYSTEM_RETENTION_LEGACY_CHANGED")
	}
	legacy, hash, e := s.legacySystemRetention(ctx, id)
	if e != nil || hash != in.LegacyDigest {
		return SystemTableRetentionMutation{}, ErrStateUnavailable
	}
	for _, r := range legacy.Runs {
		if r.TableID == id && r.State == "unconfirmed" {
			return SystemTableRetentionMutation{}, controlError(409, "SYSTEM_RETENTION_LEGACY_RESULT_UNCONFIRMED")
		}
	}
	if s.systemRetentionSource(id) == "" {
		return SystemTableRetentionMutation{}, ErrStateUnavailable
	}
	return s.saveSystemRetentionControl(ctx, id, in.RequestID, 0, systemRetentionControlDocument{nil, 1, id, hash, s.systemRetentionSource(id), systemRetentionSettings(p), false, op})
}
func (s *Service) ChangeSystemTableRetention(ctx context.Context, id string, in SystemTableRetentionChange) (SystemTableRetentionMutation, error) {
	if !in.Confirm || !validTargetUUID(in.RequestID) || in.ExpectedRevision < 1 || in.Action != "save" && in.Action != "status" || in.Action == "save" && (in.Settings == nil || in.Enabled != nil || !validSystemTablePolicy(in.Settings.policy())) || in.Action == "status" && (in.Enabled == nil || in.Settings != nil) {
		return SystemTableRetentionMutation{}, ErrPolicyInvalid
	}
	op := systemRetentionOperation{Action: "change", Change: &in}
	if out, ok, e := s.systemRetentionReplay(ctx, id, in.RequestID, op); ok {
		return out, e
	}
	_, c, e := s.systemRetentionWritable(ctx, id)
	if e != nil {
		return SystemTableRetentionMutation{}, e
	}
	if c.Authority != "database" {
		return SystemTableRetentionMutation{}, controlError(428, "SYSTEM_RETENTION_ADOPTION_REQUIRED")
	}
	if c.DesiredRevision != in.ExpectedRevision {
		return SystemTableRetentionMutation{}, controlError(409, "STORAGE_CONTROL_REVISION_CONFLICT")
	}
	state, e := s.opts.Control.read(ctx, systemRetentionDomain(id))
	if e != nil {
		return SystemTableRetentionMutation{}, e
	}
	if state.DesiredRevision != in.ExpectedRevision {
		return SystemTableRetentionMutation{}, controlError(409, "STORAGE_CONTROL_REVISION_CONFLICT")
	}
	d, e := decodeSystemRetentionControl(state.Document, id)
	if e != nil {
		return SystemTableRetentionMutation{}, e
	}
	if in.Action == "save" {
		d.Settings = *in.Settings
		d.Enabled = false
		d.Source = s.systemRetentionSource(id)
	} else {
		if *in.Enabled {
			if !s.opts.DatabaseRetentionAllowed || !s.opts.LifecycleWorkerEnabled || d.Source != s.systemRetentionSource(id) || !s.systemRetentionLedger(id).Ready(ctx) {
				return SystemTableRetentionMutation{}, ErrStateUnavailable
			}
			if e = s.systemRetention.Ready(ctx, id); e != nil {
				return SystemTableRetentionMutation{}, e
			}
		}
		d.Enabled = *in.Enabled
	}
	d.Operation = op
	return s.saveSystemRetentionControl(ctx, id, in.RequestID, in.ExpectedRevision, d)
}
func (s *Service) saveSystemRetentionControl(ctx context.Context, id, key string, revision int64, d systemRetentionControlDocument) (SystemTableRetentionMutation, error) {
	raw, e := json.Marshal(d)
	if e != nil {
		return SystemTableRetentionMutation{}, e
	}
	_, e = s.opts.Control.Save(ctx, ControlInput{key, revision, systemRetentionDomain(id), raw})
	if e != nil {
		return SystemTableRetentionMutation{}, e
	}
	return s.SystemTableRetentionRequest(ctx, id, key)
}
func (s *Service) systemRetentionReplay(ctx context.Context, id, key string, op systemRetentionOperation) (SystemTableRetentionMutation, bool, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return SystemTableRetentionMutation{}, true, ErrRootRequired
	}
	if !repo.SystemRetentionSupported(id) {
		return SystemTableRetentionMutation{}, true, ErrSystemTableNotFound
	}
	if s.opts.Control == nil {
		return SystemTableRetentionMutation{}, true, ErrStateUnavailable
	}
	v, e := s.opts.Control.repo.Request(ctx, s.opts.Control.opts.Scope.ID(), key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return SystemTableRetentionMutation{}, false, nil
	}
	if e != nil {
		return SystemTableRetentionMutation{}, true, ErrStateUnavailable
	}
	d, e := decodeSystemRetentionControl(v.Document, id)
	a, _ := json.Marshal(op)
	b, _ := json.Marshal(d.Operation)
	if e != nil || v.Domain != string(systemRetentionDomain(id)) || v.ActorID != reqctx.GetUserID(ctx) || !bytes.Equal(a, b) {
		return SystemTableRetentionMutation{}, true, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
	}
	out, e := s.SystemTableRetentionRequest(ctx, id, key)
	return out, true, e
}
func (s *Service) SystemTableRetentionRequest(ctx context.Context, id, key string) (SystemTableRetentionMutation, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return SystemTableRetentionMutation{}, ErrRootRequired
	}
	if !repo.SystemRetentionSupported(id) {
		return SystemTableRetentionMutation{}, ErrSystemTableNotFound
	}
	if s.opts.Control == nil || !validTargetUUID(key) {
		return SystemTableRetentionMutation{}, ErrPolicyInvalid
	}
	v, e := s.opts.Control.repo.Request(ctx, s.opts.Control.opts.Scope.ID(), key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return SystemTableRetentionMutation{}, controlError(404, "STORAGE_CONTROL_REQUEST_NOT_FOUND")
	}
	if e != nil {
		return SystemTableRetentionMutation{}, ErrStateUnavailable
	}
	if v.Domain != string(systemRetentionDomain(id)) || v.ActorID != reqctx.GetUserID(ctx) {
		return SystemTableRetentionMutation{}, controlError(403, "STORAGE_CONTROL_REQUEST_FORBIDDEN")
	}
	if _, e = verifiedControlDocument(v.Document, v.Digest); e != nil {
		return SystemTableRetentionMutation{}, e
	}
	_, c, _ := s.loadSystemRetentionControl(ctx, id)
	return SystemTableRetentionMutation{id, key, v.Revision, c}, nil
}
