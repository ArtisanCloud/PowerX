package storagegov

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	lifecycle "github.com/ArtisanCloud/PowerX/internal/infra/storage_lifecycle"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type LifecycleSettings struct {
	Enabled       bool   `json:"enabled"`
	CleanupMode   string `json:"cleanup_mode"`
	RetentionDays int    `json:"retention_days"`
	MaxBytes      int64  `json:"max_bytes"`
}

func lifecycleSettings(p lifecycle.Policy) LifecycleSettings {
	return LifecycleSettings{p.Enabled, p.CleanupMode, p.RetentionDays, p.MaxBytes}
}
func (p LifecycleSettings) policy() lifecycle.Policy {
	return lifecycle.Policy{Enabled: p.Enabled, CleanupMode: p.CleanupMode, RetentionDays: p.RetentionDays, MaxBytes: p.MaxBytes, RequireArchive: p.CleanupMode == "archive_then_delete"}
}

type lifecycleOrigin struct {
	Policy lifecycle.Policy `json:"policy"`
	Digest string           `json:"digest"`
}
type lifecycleDocument struct {
	Recovery    *RecoveryStamp    `json:"recovery,omitempty"`
	Version     int               `json:"version"`
	SourceID    string            `json:"source_id"`
	Fingerprint string            `json:"fingerprint"`
	Settings    LifecycleSettings `json:"settings"`
	Origin      lifecycleOrigin   `json:"origin"`
}
type LifecycleConfiguration struct {
	SourceID         string             `json:"source_id"`
	Authority        string             `json:"authority"`
	Ready            bool               `json:"ready"`
	CanEnable        bool               `json:"can_enable"`
	State            string             `json:"state"`
	DesiredRevision  int64              `json:"desired_revision"`
	AppliedRevision  int64              `json:"applied_revision"`
	SnapshotRevision *int64             `json:"snapshot_revision"`
	DesiredPolicy    *LifecycleSettings `json:"desired_policy"`
	LegacyRevision   int64              `json:"legacy_revision"`
	LegacyDigest     string             `json:"legacy_digest"`
	LegacyEnabled    bool               `json:"legacy_enabled"`
	Problem          string             `json:"problem,omitempty"`
}
type LifecycleMutation struct {
	SourceID          string                 `json:"source_id"`
	RequestID         string                 `json:"request_id"`
	OperationRevision int64                  `json:"operation_revision"`
	OperationPolicy   LifecycleSettings      `json:"operation_policy"`
	Configuration     LifecycleConfiguration `json:"configuration"`
}

func lifecycleDomain(id string) ControlDomain {
	return ControlDomain("file-lifecycle." + controlDigest([]byte(id))[:32])
}
func decodeLifecycleDocument(raw []byte) (lifecycleDocument, error) {
	var d lifecycleDocument
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&d) != nil || decoder.Decode(new(any)) != io.EOF || d.Version != 1 || !idPattern.MatchString(d.SourceID) || len(d.Fingerprint) != 64 || len(d.Origin.Digest) != 64 || d.Settings.RetentionDays < 1 || d.Settings.RetentionDays > 3650 || d.Settings.MaxBytes < 1 || d.Settings.MaxBytes > 1<<50 || d.Settings.CleanupMode != "disabled" && d.Settings.CleanupMode != "expire_delete" && d.Settings.CleanupMode != "archive_then_delete" || d.Settings.Enabled && d.Settings.CleanupMode == "disabled" {
		return d, ErrPolicyInvalid
	}
	return d, nil
}
func lifecycleLegacyDigest(p lifecycle.Policy) (string, error) {
	raw, e := json.Marshal(p)
	if e != nil {
		return "", e
	}
	raw, e = canonicalControl(raw)
	if e != nil {
		return "", e
	}
	return controlDigest(raw), nil
}
func (s *Service) bindLifecycleControl() error {
	if s.opts.Control == nil || s.opts.Lifecycle == nil {
		return nil
	}
	for _, source := range s.opts.Lifecycle.RegisteredSources() {
		if !source.Supported {
			continue
		}
		source := source
		if err := s.opts.Control.Bind(lifecycleDomain(source.ID), ControlBinding{Validate: func(raw json.RawMessage) error {
			d, e := decodeLifecycleDocument(raw)
			if e != nil {
				return e
			}
			if d.SourceID != source.ID || d.Fingerprint != source.Fingerprint {
				return ErrPolicyInvalid
			}
			return nil
		}, Apply: func(ctx context.Context, p ControlPublication) error {
			d, e := decodeLifecycleDocument(p.Document)
			if e != nil {
				return e
			}
			if d.SourceID != source.ID || d.Fingerprint != source.Fingerprint {
				return lifecycle.ErrBlocked
			}
			if p.Revision == 1 {
				legacy, _, e := s.opts.Lifecycle.PolicySnapshot(ctx, source.ID)
				if e != nil {
					return e
				}
				digest, e := lifecycleLegacyDigest(legacy)
				if e != nil {
					return e
				}
				if digest != d.Origin.Digest {
					return controlError(409, "STORAGE_LIFECYCLE_LEGACY_CHANGED")
				}
			}
			policy, e := s.opts.Lifecycle.ValidatePolicy(ctx, p.ActorID, source.ID, p.Revision, d.Settings.policy())
			if e != nil {
				return e
			}
			policy.UpdatedAt = p.UpdatedAt
			policy.ConfigurationDigest = p.Digest
			return s.opts.Lifecycle.PublishPolicy(ctx, source.ID, lifecycle.Publication{Version: 1, HostID: p.Scope.HostID, FilesystemID: p.Scope.FilesystemID, InstanceID: p.Scope.InstanceID, Digest: p.Digest, Policy: policy})
		}}); err != nil {
			return err
		}
	}
	return s.opts.Lifecycle.BindAuthority(lifecycle.Authority{Resolve: func(ctx context.Context, source lifecycle.Source, legacy lifecycle.Policy, published *lifecycle.Publication) (lifecycle.Policy, error) {
		p, _, e := s.resolveLifecycle(ctx, source, legacy, published)
		return p, e
	}, Fence: s.fenceLifecycle})
}
func (s *Service) resolveLifecycle(ctx context.Context, source lifecycle.Source, legacy lifecycle.Policy, published *lifecycle.Publication) (lifecycle.Policy, LifecycleConfiguration, error) {
	p := legacy
	c := LifecycleConfiguration{SourceID: source.ID, Authority: "unavailable", State: "unavailable"}
	fail := func(code string) (lifecycle.Policy, LifecycleConfiguration, error) {
		p.Enabled = false
		c.Problem = code
		return p, c, lifecycle.ErrBlocked
	}
	if s.opts.Control == nil {
		return fail("STORAGE_CONTROL_SCHEMA_NOT_READY")
	}
	state, e := s.opts.Control.read(ctx, lifecycleDomain(source.ID))
	if e != nil {
		return fail("STORAGE_CONTROL_UNAVAILABLE")
	}
	c.Ready = s.opts.Control.Ready(ctx)
	c.CanEnable = c.Ready && s.opts.ControlWorkerEnabled && !s.opts.Control.ConsumerExited()
	if state.State == "unconfigured" {
		if published != nil {
			return fail("STORAGE_LIFECYCLE_RESTORE_CONFIRMATION_REQUIRED")
		}
		digest, e := lifecycleLegacyDigest(legacy)
		if e != nil {
			return fail("STORAGE_LIFECYCLE_LEGACY_UNAVAILABLE")
		}
		c.Authority = "legacy"
		c.State = "legacy"
		c.LegacyRevision = legacy.Revision
		c.LegacyDigest = digest
		c.LegacyEnabled = legacy.Enabled
		if legacy.Enabled && (legacy.OwnerInstance != s.opts.Identity.InstanceID || legacy.Fingerprint != source.Fingerprint) {
			return fail("STORAGE_LIFECYCLE_SOURCE_CHANGED")
		}
		return legacy, c, nil
	}
	d, e := decodeLifecycleDocument(state.Document)
	if e != nil || d.SourceID != source.ID || d.Fingerprint != source.Fingerprint {
		return fail("STORAGE_LIFECYCLE_SOURCE_CHANGED")
	}
	c.Authority = "database"
	c.State = state.State
	c.DesiredRevision = state.DesiredRevision
	c.AppliedRevision = state.AppliedRevision
	c.DesiredPolicy = &d.Settings
	c.LegacyRevision = d.Origin.Policy.Revision
	c.LegacyDigest = d.Origin.Digest
	c.LegacyEnabled = d.Origin.Policy.Enabled
	c.Problem = state.Problem
	if published == nil && state.AppliedRevision == 0 {
		p.Enabled = false
		return p, c, nil
	}
	if published == nil || published.Version != 1 || published.HostID != state.Scope.HostID || published.FilesystemID != state.Scope.FilesystemID || published.InstanceID != state.Scope.InstanceID || published.Policy.Revision != state.AppliedRevision || published.Policy.SourceID != source.ID || published.Policy.OwnerInstance != state.Scope.InstanceID || published.Policy.Fingerprint != source.Fingerprint {
		c.State = "unconfirmed"
		return fail("STORAGE_LIFECYCLE_SNAPSHOT_UNCONFIRMED")
	}
	p = published.Policy
	c.SnapshotRevision = &p.Revision
	version, e := s.opts.Control.repo.Revision(ctx, state.Scope.ID(), string(lifecycleDomain(source.ID)), p.Revision)
	if e != nil || version.Digest != published.Digest || p.ConfigurationDigest != version.Digest || version.ActorID != p.OperatorID || !version.CreatedAt.Equal(p.UpdatedAt) {
		c.State = "unconfirmed"
		return fail("STORAGE_LIFECYCLE_SNAPSHOT_UNCONFIRMED")
	}
	body, e := verifiedControlDocument(version.Document, version.Digest)
	if e != nil {
		c.State = "unconfirmed"
		return fail("STORAGE_LIFECYCLE_SNAPSHOT_UNCONFIRMED")
	}
	approved, e := decodeLifecycleDocument(body)
	if e != nil || lifecycleSettings(p) != approved.Settings || approved.SourceID != source.ID || approved.Fingerprint != source.Fingerprint {
		c.State = "unconfirmed"
		return fail("STORAGE_LIFECYCLE_SNAPSHOT_UNCONFIRMED")
	}
	if state.DesiredRevision != state.AppliedRevision || state.State != "applied" {
		p.Enabled = false
	}
	return p, c, nil
}
func (s *Service) fenceLifecycle(ctx context.Context, source lifecycle.Source, p lifecycle.Policy, execute func() error) error {
	if s.opts.Control == nil {
		return lifecycle.ErrBlocked
	}
	return s.opts.Control.WithExecution(ctx, lifecycleDomain(source.ID), func(row *m.ControlConfiguration) error {
		if row == nil {
			if p.ConfigurationDigest != "" {
				return lifecycle.ErrBlocked
			}
			return execute()
		}
		if row.Revision != p.Revision || row.AppliedRevision != p.Revision || row.PublicationState != "applied" || row.Digest != p.ConfigurationDigest {
			return lifecycle.ErrConflict
		}
		body, e := verifiedControlDocument(row.Document, row.Digest)
		if e != nil {
			return lifecycle.ErrBlocked
		}
		d, e := decodeLifecycleDocument(body)
		if e != nil || !d.Settings.Enabled || d.SourceID != source.ID || d.Fingerprint != source.Fingerprint || lifecycleSettings(p) != d.Settings || row.HostID != s.opts.Identity.HostID || row.FilesystemID != s.opts.Identity.FilesystemID || row.InstanceID != s.opts.Identity.InstanceID {
			return lifecycle.ErrBlocked
		}
		return execute()
	})
}
func (s *Service) lifecycleConfiguration(ctx context.Context, id string) (LifecycleConfiguration, error) {
	if s.opts.Lifecycle == nil {
		return LifecycleConfiguration{}, ErrStateUnavailable
	}
	source, ok := s.opts.Lifecycle.Source(id)
	if !ok || !source.Supported {
		return LifecycleConfiguration{}, ErrPolicyInvalid
	}
	legacy, published, e := s.opts.Lifecycle.PolicySnapshot(ctx, id)
	if e != nil {
		return LifecycleConfiguration{}, e
	}
	_, c, _ := s.resolveLifecycle(ctx, source, legacy, published)
	return c, nil
}
func (s *Service) LifecycleConfiguration(ctx context.Context, id string) (LifecycleConfiguration, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return LifecycleConfiguration{}, ErrRootRequired
	}
	return s.lifecycleConfiguration(ctx, id)
}
func (s *Service) AdoptLifecycle(ctx context.Context, id string, revision int64, digest, key string) (LifecycleMutation, error) {
	if e := s.requireLifecycleWrite(ctx, id); e != nil {
		return LifecycleMutation{}, e
	}
	if _, e := uuid.Parse(key); e != nil || len(key) != 36 {
		return LifecycleMutation{}, ErrPolicyInvalid
	}
	source, _ := s.opts.Lifecycle.Source(id)
	old, e := s.opts.Control.repo.Request(ctx, s.opts.Control.opts.Scope.ID(), key)
	if e == nil {
		if old.Domain != string(lifecycleDomain(id)) {
			return LifecycleMutation{}, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
		}
		body, e := verifiedControlDocument(old.Document, old.Digest)
		if e != nil {
			return LifecycleMutation{}, e
		}
		d, e := decodeLifecycleDocument(body)
		if e != nil {
			return LifecycleMutation{}, e
		}
		if d.Origin.Policy.Revision != revision || d.Origin.Digest != digest {
			return LifecycleMutation{}, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
		}
		return s.saveLifecycleDocument(ctx, id, 0, key, d)
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return LifecycleMutation{}, ErrStateUnavailable
	}
	legacy, _, e := s.opts.Lifecycle.PolicySnapshot(ctx, id)
	if e != nil {
		return LifecycleMutation{}, e
	}
	actual, e := lifecycleLegacyDigest(legacy)
	if e != nil {
		return LifecycleMutation{}, e
	}
	if legacy.Revision != revision || actual != digest {
		return LifecycleMutation{}, controlError(409, "STORAGE_LIFECYCLE_LEGACY_CHANGED")
	}
	if legacy.Enabled && legacy.OwnerInstance != s.opts.Identity.InstanceID {
		return LifecycleMutation{}, lifecycle.ErrBlocked
	}
	d := lifecycleDocument{Version: 1, SourceID: id, Fingerprint: source.Fingerprint, Settings: lifecycleSettings(legacy), Origin: lifecycleOrigin{legacy, actual}}
	d.Settings.Enabled = false
	return s.saveLifecycleDocument(ctx, id, 0, key, d)
}
func (s *Service) requireLifecycleWrite(ctx context.Context, id string) error {
	if !auth.IsPlatformRootSession(ctx) {
		return ErrRootRequired
	}
	if s.opts.Control == nil || !s.opts.Control.Ready(ctx) || s.opts.Lifecycle == nil {
		return ErrStateUnavailable
	}
	source, ok := s.opts.Lifecycle.Source(id)
	if !ok || !source.Supported {
		return ErrPolicyInvalid
	}
	return nil
}
func (s *Service) SaveLifecycleConfiguration(ctx context.Context, id string, revision int64, key string, p LifecycleSettings) (LifecycleMutation, error) {
	if e := s.requireLifecycleWrite(ctx, id); e != nil {
		return LifecycleMutation{}, e
	}
	if revision < 1 {
		return LifecycleMutation{}, controlError(409, "STORAGE_LIFECYCLE_ADOPTION_REQUIRED")
	}
	// 已提交请求按不可变回执核对；运行条件改变不能把同一请求变成第二次写入。
	if _, e := uuid.Parse(key); e != nil || len(key) != 36 {
		return LifecycleMutation{}, ErrPolicyInvalid
	}
	old, e := s.opts.Control.repo.Request(ctx, s.opts.Control.opts.Scope.ID(), key)
	if e == nil {
		body, err := verifiedControlDocument(old.Document, old.Digest)
		if err != nil {
			return LifecycleMutation{}, err
		}
		d, err := decodeLifecycleDocument(body)
		if err != nil {
			return LifecycleMutation{}, err
		}
		if old.Domain != string(lifecycleDomain(id)) || old.Revision != revision+1 || d.Settings != p {
			return LifecycleMutation{}, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
		}
		return s.saveLifecycleDocument(ctx, id, revision, key, d)
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return LifecycleMutation{}, ErrStateUnavailable
	}
	state, e := s.opts.Control.read(ctx, lifecycleDomain(id))
	if e != nil {
		return LifecycleMutation{}, e
	}
	if state.State == "unconfigured" {
		return LifecycleMutation{}, controlError(409, "STORAGE_LIFECYCLE_ADOPTION_REQUIRED")
	}
	d, e := decodeLifecycleDocument(state.Document)
	if e != nil {
		return LifecycleMutation{}, e
	}
	d.Settings = p
	if p.Enabled && (!s.opts.ControlWorkerEnabled || s.opts.Control.ConsumerExited()) {
		return LifecycleMutation{}, controlError(503, "STORAGE_CONTROL_WORKER_UNAVAILABLE")
	}
	if _, e = s.opts.Lifecycle.ValidatePolicy(ctx, reqctx.GetClaims(ctx).UserID, id, revision+1, p.policy()); e != nil {
		return LifecycleMutation{}, e
	}
	return s.saveLifecycleDocument(ctx, id, revision, key, d)
}
func (s *Service) saveLifecycleDocument(ctx context.Context, id string, revision int64, key string, d lifecycleDocument) (LifecycleMutation, error) {
	body, e := json.Marshal(d)
	if e != nil {
		return LifecycleMutation{}, e
	}
	state, e := s.opts.Control.Save(ctx, ControlInput{RequestID: key, ExpectedRevision: revision, Domain: lifecycleDomain(id), Document: body})
	if e != nil {
		return LifecycleMutation{}, e
	}
	c, e := s.lifecycleConfiguration(ctx, id)
	if e != nil {
		return LifecycleMutation{}, e
	}
	return LifecycleMutation{id, key, state.OperationRevision, d.Settings, c}, nil
}
func (s *Service) LifecycleRequest(ctx context.Context, id, key string) (LifecycleMutation, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return LifecycleMutation{}, ErrRootRequired
	}
	if s.opts.Control == nil {
		return LifecycleMutation{}, ErrStateUnavailable
	}
	if _, e := uuid.Parse(key); e != nil || len(key) != 36 {
		return LifecycleMutation{}, ErrPolicyInvalid
	}
	row, e := s.opts.Control.repo.Request(ctx, s.opts.Control.opts.Scope.ID(), key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return LifecycleMutation{}, controlError(404, "STORAGE_CONTROL_REQUEST_NOT_FOUND")
	}
	if e != nil {
		return LifecycleMutation{}, ErrStateUnavailable
	}
	if row.Domain != string(lifecycleDomain(id)) || row.ActorID != reqctx.GetClaims(ctx).UserID {
		return LifecycleMutation{}, controlError(403, "STORAGE_CONTROL_REQUEST_FORBIDDEN")
	}
	body, e := verifiedControlDocument(row.Document, row.Digest)
	if e != nil {
		return LifecycleMutation{}, e
	}
	d, e := decodeLifecycleDocument(body)
	if e != nil {
		return LifecycleMutation{}, e
	}
	c, e := s.lifecycleConfiguration(ctx, id)
	if e != nil {
		return LifecycleMutation{}, e
	}
	return LifecycleMutation{id, key, row.Revision, d.Settings, c}, nil
}
