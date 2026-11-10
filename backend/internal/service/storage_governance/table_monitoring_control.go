package storagegov

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TableMonitoringSettings 仅包含用户可编辑阈值，版本、身份及审计由服务端维护。
type TableMonitoringSettings struct {
	Enabled             bool  `json:"enabled"`
	WarningBytes        int64 `json:"warning_bytes"`
	GrowthBytes         int64 `json:"growth_bytes"`
	GrowthWindowMinutes int   `json:"growth_window_minutes"`
	StaleAfterMinutes   int   `json:"stale_after_minutes"`
	FailureLimit        int   `json:"failure_limit"`
}

func monitoringSettings(p TableMonitoringPolicy) TableMonitoringSettings {
	return TableMonitoringSettings{p.Enabled, p.WarningBytes, p.GrowthBytes, p.GrowthWindowMinutes, p.StaleAfterMinutes, p.FailureLimit}
}
func (p TableMonitoringSettings) policy() TableMonitoringPolicy {
	return TableMonitoringPolicy{Enabled: p.Enabled, WarningBytes: p.WarningBytes, GrowthBytes: p.GrowthBytes, GrowthWindowMinutes: p.GrowthWindowMinutes, StaleAfterMinutes: p.StaleAfterMinutes, FailureLimit: p.FailureLimit}
}

type monitoringOrigin struct {
	Revision int64  `json:"revision"`
	Digest   string `json:"digest"`
	Enabled  bool   `json:"enabled"`
}
type monitoringDocument struct {
	Recovery *RecoveryStamp          `json:"recovery,omitempty"`
	Version  int                     `json:"version"`
	Settings TableMonitoringSettings `json:"settings"`
	Origin   monitoringOrigin        `json:"origin"`
}
type monitoringProjection struct {
	Version        int             `json:"version"`
	Scope          ControlScope    `json:"scope"`
	Revision       int64           `json:"revision"`
	Digest         string          `json:"digest"`
	Document       json.RawMessage `json:"document"`
	ActorID        uint64          `json:"actor_id"`
	UpdatedAt      time.Time       `json:"updated_at"`
	OfflineAllowed bool            `json:"offline_allowed"`
}

// TableMonitoringConfiguration 如实区分旧配置、数据库期望版本与本地确认版本。
type TableMonitoringConfiguration struct {
	Authority        string                   `json:"authority"`
	Ready            bool                     `json:"ready"`
	State            string                   `json:"state"`
	DesiredRevision  int64                    `json:"desired_revision"`
	AppliedRevision  int64                    `json:"applied_revision"`
	SnapshotRevision *int64                   `json:"snapshot_revision"`
	DesiredPolicy    *TableMonitoringSettings `json:"desired_policy"`
	LegacyRevision   int64                    `json:"legacy_revision"`
	LegacyDigest     string                   `json:"legacy_digest"`
	LegacyEnabled    bool                     `json:"legacy_enabled"`
	Problem          string                   `json:"problem,omitempty"`
}
type TableMonitoringMutation struct {
	RequestID         string                       `json:"request_id"`
	OperationRevision int64                        `json:"operation_revision"`
	OperationPolicy   TableMonitoringSettings      `json:"operation_policy"`
	Configuration     TableMonitoringConfiguration `json:"configuration"`
}

func strictMonitoringDocument(raw []byte) (monitoringDocument, error) {
	var doc monitoringDocument
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&doc) != nil || d.Decode(new(any)) != io.EOF || doc.Version != 1 || !validTableMonitoringPolicy(doc.Settings.policy()) || doc.Origin.Revision < 0 || len(doc.Origin.Digest) != 64 {
		return doc, controlError(400, "STORAGE_TABLE_MONITORING_DOCUMENT_INVALID")
	}
	return doc, nil
}
func (s *Service) legacyMonitoring(ctx context.Context) (TableMonitoringPolicy, string, error) {
	if s.opts.Budget == nil {
		return TableMonitoringPolicy{}, "", ErrStateUnavailable
	}
	raw, e := s.opts.Budget.Document(ctx, "table-monitoring", nil)
	if e != nil {
		return TableMonitoringPolicy{}, "", e
	}
	p, e := s.tableMonitoringPolicy(raw)
	if e != nil {
		return p, "", e
	}
	// 空文件也有明确默认值；摘要绑定全部旧元数据，保留来源证据。
	body, e := json.Marshal(p)
	if e != nil {
		return p, "", e
	}
	body, e = canonicalControl(body)
	if e != nil {
		return p, "", e
	}
	return p, controlDigest(body), nil
}
func (s *Service) monitoringBinding() ControlBinding {
	return ControlBinding{Validate: func(raw json.RawMessage) error { _, e := strictMonitoringDocument(raw); return e }, Apply: func(ctx context.Context, p ControlPublication) error {
		if s.opts.Budget == nil || p.Scope != (ControlScope{s.opts.Identity.HostID, s.opts.Identity.FilesystemID, s.opts.Identity.InstanceID}) {
			return ErrStateUnavailable
		}
		doc, e := strictMonitoringDocument(p.Document)
		if e != nil {
			return e
		}
		if p.Revision == 1 {
			_, digest, e := s.legacyMonitoring(ctx)
			if e != nil {
				return e
			}
			if digest != doc.Origin.Digest {
				return controlError(409, "STORAGE_TABLE_MONITORING_LEGACY_CHANGED")
			}
		}
		_, e = s.opts.Budget.Document(ctx, "table-monitoring-control", func(raw []byte) ([]byte, error) {
			if len(raw) > 0 {
				var old monitoringProjection
				if json.Unmarshal(raw, &old) != nil || old.Version != 1 || old.Scope != p.Scope || old.OfflineAllowed || old.Revision > p.Revision || old.Revision == p.Revision && old.Digest != p.Digest {
					return nil, ErrStateUnavailable
				}
			}
			return json.Marshal(monitoringProjection{Version: 1, Scope: p.Scope, Revision: p.Revision, Digest: p.Digest, Document: p.Document, ActorID: p.ActorID, UpdatedAt: p.UpdatedAt, OfflineAllowed: false})
		})
		return e
	}}
}

func (s *Service) monitoringControl(ctx context.Context) (TableMonitoringPolicy, TableMonitoringConfiguration, error) {
	p, _ := s.tableMonitoringPolicy(nil)
	c := TableMonitoringConfiguration{Authority: "unavailable", State: "unavailable"}
	fail := func(code string) (TableMonitoringPolicy, TableMonitoringConfiguration, error) {
		p.Enabled = false
		c.Problem = code
		return p, c, ErrStateUnavailable
	}
	if s.opts.Control == nil || s.opts.Budget == nil || !s.opts.Control.Ready(ctx) {
		return fail("STORAGE_CONTROL_SCHEMA_NOT_READY")
	}
	c.Ready = s.opts.ControlWorkerEnabled && !s.opts.Control.ConsumerExited()
	if s.opts.Control.ConsumerExited() {
		c.Problem = "STORAGE_CONTROL_WORKER_UNAVAILABLE"
	}
	state, e := s.opts.Control.read(ctx, ControlTableMonitoring)
	if e != nil {
		return fail("STORAGE_CONTROL_UNAVAILABLE")
	}
	raw, e := s.opts.Budget.Document(ctx, "table-monitoring-control", nil)
	if e != nil {
		return fail("STORAGE_TABLE_MONITORING_SNAPSHOT_UNAVAILABLE")
	}
	if state.State == "unconfigured" {
		if len(raw) > 0 {
			return fail("STORAGE_TABLE_MONITORING_RESTORE_CONFIRMATION_REQUIRED")
		}
		legacy, digest, e := s.legacyMonitoring(ctx)
		if e != nil {
			return fail("STORAGE_TABLE_MONITORING_LEGACY_UNAVAILABLE")
		}
		c.Authority = "legacy"
		c.State = "legacy"
		c.LegacyRevision = legacy.Revision
		c.LegacyDigest = digest
		c.LegacyEnabled = legacy.Enabled
		return legacy, c, nil
	}
	doc, e := strictMonitoringDocument(state.Document)
	if e != nil {
		return fail("STORAGE_TABLE_MONITORING_DOCUMENT_INVALID")
	}
	c.Authority = "database"
	c.State = state.State
	c.DesiredRevision = state.DesiredRevision
	c.AppliedRevision = state.AppliedRevision
	c.DesiredPolicy = &doc.Settings
	c.LegacyRevision = doc.Origin.Revision
	c.LegacyDigest = doc.Origin.Digest
	c.LegacyEnabled = doc.Origin.Enabled
	c.Problem = state.Problem
	if s.opts.Control.ConsumerExited() {
		c.Problem = "STORAGE_CONTROL_WORKER_UNAVAILABLE"
	}
	if len(raw) == 0 && state.AppliedRevision == 0 {
		return p, c, nil
	}
	var projection monitoringProjection
	if json.Unmarshal(raw, &projection) != nil || projection.Version != 1 || projection.Scope != state.Scope || projection.OfflineAllowed || projection.Revision != state.AppliedRevision || projection.Revision < 1 {
		c.State = "unconfirmed"
		return fail("STORAGE_TABLE_MONITORING_SNAPSHOT_UNCONFIRMED")
	}
	c.SnapshotRevision = &projection.Revision
	version, e := s.opts.Control.repo.Revision(ctx, state.Scope.ID(), string(ControlTableMonitoring), projection.Revision)
	if e != nil || version.Digest != projection.Digest || version.ActorID != projection.ActorID || !version.CreatedAt.Equal(projection.UpdatedAt) {
		c.State = "unconfirmed"
		return fail("STORAGE_TABLE_MONITORING_SNAPSHOT_UNCONFIRMED")
	}
	if _, e = verifiedControlDocument(projection.Document, version.Digest); e != nil {
		c.State = "unconfirmed"
		return fail("STORAGE_TABLE_MONITORING_SNAPSHOT_UNCONFIRMED")
	}
	if _, e = verifiedControlDocument(version.Document, version.Digest); e != nil {
		c.State = "unconfirmed"
		return fail("STORAGE_TABLE_MONITORING_SNAPSHOT_UNCONFIRMED")
	}
	applied, e := strictMonitoringDocument(projection.Document)
	if e != nil {
		return fail("STORAGE_TABLE_MONITORING_DOCUMENT_INVALID")
	}
	p = applied.Settings.policy()
	p.Revision = projection.Revision
	p.OwnerInstance = state.Scope.InstanceID
	p.Actor = projection.ActorID
	p.UpdatedAt = projection.UpdatedAt
	// 停用期望值立即阻止本节点告警，不能等待旧启用快照再次发布。
	if !doc.Settings.Enabled {
		p.Enabled = false
	}
	return p, c, nil
}
func (s *Service) requireMonitoringWrite(ctx context.Context) error {
	if !auth.IsPlatformRootSession(ctx) {
		return ErrRootRequired
	}
	if s.opts.Control == nil || s.opts.Budget == nil || !s.opts.Control.Ready(ctx) || !s.opts.ControlWorkerEnabled {
		return controlError(503, "STORAGE_CONTROL_SCHEMA_NOT_READY")
	}
	if s.opts.Control.ConsumerExited() {
		return controlError(503, "STORAGE_CONTROL_WORKER_UNAVAILABLE")
	}
	return nil
}
func (s *Service) validMonitoringSettings(p TableMonitoringSettings) error {
	if !validTableMonitoringPolicy(p.policy()) {
		return ErrPolicyInvalid
	}
	if p.Enabled {
		_, metrics := s.opts.DatabaseCollector.(SystemTableMetricsCollector)
		if !metrics || !s.stateReady() || !s.opts.SamplingConfigured || !s.opts.SamplingWorkerEnabled {
			return ErrStateUnavailable
		}
	}
	return nil
}

// AdoptTableMonitoring 明确接管旧配置并暂停告警，保留原启用值与摘要供审计。
func (s *Service) AdoptTableMonitoring(ctx context.Context, revision int64, digest, key string) (TableMonitoringMutation, error) {
	if e := s.requireMonitoringWrite(ctx); e != nil {
		return TableMonitoringMutation{}, e
	}
	if _, e := uuid.Parse(key); e != nil || len(key) != 36 {
		return TableMonitoringMutation{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	previous, e := s.opts.Control.repo.Request(ctx, s.opts.Control.opts.Scope.ID(), key)
	if e == nil {
		if previous.Domain != string(ControlTableMonitoring) {
			return TableMonitoringMutation{}, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
		}
		body, e := verifiedControlDocument(previous.Document, previous.Digest)
		if e != nil {
			return TableMonitoringMutation{}, e
		}
		doc, e := strictMonitoringDocument(body)
		if e != nil {
			return TableMonitoringMutation{}, e
		}
		if doc.Origin.Revision != revision || doc.Origin.Digest != digest {
			return TableMonitoringMutation{}, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
		}
		return s.saveMonitoringDocument(ctx, 0, key, doc)
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return TableMonitoringMutation{}, controlError(503, "STORAGE_CONTROL_UNAVAILABLE")
	}
	p, actual, e := s.legacyMonitoring(ctx)
	if e != nil {
		return TableMonitoringMutation{}, e
	}
	if revision != p.Revision || actual != digest {
		return TableMonitoringMutation{}, controlError(409, "STORAGE_TABLE_MONITORING_LEGACY_CHANGED")
	}
	doc := monitoringDocument{Version: 1, Settings: monitoringSettings(p), Origin: monitoringOrigin{p.Revision, actual, p.Enabled}}
	doc.Settings.Enabled = false
	return s.saveMonitoringDocument(ctx, 0, key, doc)
}
func (s *Service) SaveTableMonitoringConfiguration(ctx context.Context, revision int64, key string, p TableMonitoringSettings) (TableMonitoringMutation, error) {
	if e := s.requireMonitoringWrite(ctx); e != nil {
		return TableMonitoringMutation{}, e
	}
	if revision < 1 {
		return TableMonitoringMutation{}, controlError(409, "STORAGE_TABLE_MONITORING_ADOPTION_REQUIRED")
	}
	if e := s.validMonitoringSettings(p); e != nil {
		return TableMonitoringMutation{}, e
	}
	state, e := s.opts.Control.read(ctx, ControlTableMonitoring)
	if e != nil {
		return TableMonitoringMutation{}, e
	}
	if state.State == "unconfigured" {
		return TableMonitoringMutation{}, controlError(409, "STORAGE_TABLE_MONITORING_ADOPTION_REQUIRED")
	}
	doc, e := strictMonitoringDocument(state.Document)
	if e != nil {
		return TableMonitoringMutation{}, e
	}
	doc.Settings = p
	return s.saveMonitoringDocument(ctx, revision, key, doc)
}
func (s *Service) saveMonitoringDocument(ctx context.Context, revision int64, key string, doc monitoringDocument) (TableMonitoringMutation, error) {
	body, e := json.Marshal(doc)
	if e != nil {
		return TableMonitoringMutation{}, e
	}
	state, e := s.opts.Control.Save(ctx, ControlInput{RequestID: key, ExpectedRevision: revision, Domain: ControlTableMonitoring, Document: body})
	if e != nil {
		return TableMonitoringMutation{}, e
	}
	_, configuration, _ := s.monitoringControl(ctx)
	return TableMonitoringMutation{key, state.OperationRevision, doc.Settings, configuration}, nil
}

// TableMonitoringRequest 只读核对原请求，不重放创建、保存或启用。
func (s *Service) TableMonitoringRequest(ctx context.Context, key string) (TableMonitoringMutation, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return TableMonitoringMutation{}, ErrRootRequired
	}
	if s.opts.Control == nil || !s.opts.Control.repo.Ready(ctx) {
		return TableMonitoringMutation{}, controlError(503, "STORAGE_CONTROL_SCHEMA_NOT_READY")
	}
	if _, e := uuid.Parse(key); e != nil || len(key) != 36 {
		return TableMonitoringMutation{}, ErrPolicyInvalid
	}
	row, e := s.opts.Control.repo.Request(ctx, s.opts.Control.opts.Scope.ID(), key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return TableMonitoringMutation{}, controlError(404, "STORAGE_CONTROL_REQUEST_NOT_FOUND")
	}
	if e != nil {
		return TableMonitoringMutation{}, controlError(503, "STORAGE_CONTROL_UNAVAILABLE")
	}
	if row.Domain != string(ControlTableMonitoring) || row.ActorID != reqctx.GetClaims(ctx).UserID {
		return TableMonitoringMutation{}, controlError(403, "STORAGE_CONTROL_REQUEST_FORBIDDEN")
	}
	body, e := verifiedControlDocument(row.Document, row.Digest)
	if e != nil {
		return TableMonitoringMutation{}, e
	}
	doc, e := strictMonitoringDocument(body)
	if e != nil {
		return TableMonitoringMutation{}, e
	}
	_, configuration, _ := s.monitoringControl(ctx)
	return TableMonitoringMutation{key, row.Revision, doc.Settings, configuration}, nil
}

// StartControl 使用正式 TaskBus；不启动业务清理或第二套队列。
func (s *Service) StartControl(ctx context.Context, onError func(error)) {
	if s.opts.Control != nil && s.opts.ControlWorkerEnabled {
		s.opts.Control.Start(ctx, onError)
	}
}
