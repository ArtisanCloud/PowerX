package storagegov

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	snapshot "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
	"io"
	"sort"
	"strings"
	"time"
)

type snapshotPolicyOperation struct {
	Action   string                   `json:"action"`
	Adoption *snapshot.PolicyAdoption `json:"adoption,omitempty"`
	Change   *snapshot.PolicyChange   `json:"change,omitempty"`
}
type snapshotPolicyDocument struct {
	Recovery     *RecoveryStamp          `json:"recovery,omitempty"`
	Version      int                     `json:"version"`
	Origin       snapshot.LegacyPolicies `json:"origin"`
	OriginDigest string                  `json:"origin_digest"`
	Policies     []snapshot.PolicyEntry  `json:"policies"`
	Operation    snapshotPolicyOperation `json:"operation"`
}
type snapshotPolicyProjection struct {
	Version   int             `json:"version"`
	Scope     ControlScope    `json:"scope"`
	Revision  int64           `json:"revision"`
	Digest    string          `json:"digest"`
	Document  json.RawMessage `json:"document"`
	ActorID   uint64          `json:"actor_id"`
	UpdatedAt time.Time       `json:"updated_at"`
}
type snapshotPolicyControl struct {
	service   *Service
	snapshots *snapshot.Service
}

func decodeSnapshotPolicies(raw []byte) (snapshotPolicyDocument, error) {
	var d snapshotPolicyDocument
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&d) != nil || dec.Decode(new(any)) != io.EOF || d.Version != 1 || len(d.Policies) > 100 || len(d.Origin.Policies) > 100 || len(d.OriginDigest) != 64 {
		return d, controlError(400, "SNAPSHOT_POLICY_DOCUMENT_INVALID")
	}
	origin, _ := json.Marshal(d.Origin)
	if controlDigest(origin) != d.OriginDigest {
		return d, controlError(400, "SNAPSHOT_POLICY_DOCUMENT_INVALID")
	}
	ids, names := map[string]bool{}, map[string]bool{}
	current := 0
	for _, p := range d.Policies {
		if !validTargetUUID(p.ID) || p.Revision < 1 || len(p.Fingerprint) != 64 || ids[p.ID] || names[strings.ToLower(p.Settings.Name)] || snapshot.ValidatePolicySettings(p.Settings) != nil || p.Enabled && (!p.Current || p.Anchor.IsZero()) {
			return d, controlError(400, "SNAPSHOT_POLICY_DOCUMENT_INVALID")
		}
		ids[p.ID] = true
		names[strings.ToLower(p.Settings.Name)] = true
		if p.Current {
			current++
		}
	}
	if current > 1 || (d.Operation.Action != "adopt" || d.Operation.Adoption == nil || d.Operation.Change != nil) && (d.Operation.Action != "change" || d.Operation.Change == nil || d.Operation.Adoption != nil) {
		return d, controlError(400, "SNAPSHOT_POLICY_DOCUMENT_INVALID")
	}
	return d, nil
}
func (s *Service) BindSystemSnapshotControl(snaps *snapshot.Service) error {
	if snaps == nil || s.opts.Control == nil || s.opts.DB == nil || s.opts.Budget == nil {
		return ErrStateUnavailable
	}
	c := &snapshotPolicyControl{s, snaps}
	s.recoverySnapshots = c
	if e := s.opts.Control.Bind(ControlSnapshotPolicies, ControlBinding{Validate: func(raw json.RawMessage) error { _, e := decodeSnapshotPolicies(raw); return e }, Apply: c.publish}); e != nil {
		return e
	}
	return snaps.BindPolicyAuthority(s.opts.Control.opts.Scope.ID(), c)
}
func (c *snapshotPolicyControl) publish(ctx context.Context, p ControlPublication) error {
	if p.Scope != c.service.opts.Control.opts.Scope {
		return ErrStateUnavailable
	}
	d, e := decodeSnapshotPolicies(p.Document)
	if e != nil {
		return e
	}
	return c.snapshots.PublishPolicyBinding(ctx, p.Scope.ID(), d.OriginDigest, p.Revision, func() error {
		_, err := updateArchiveControlProjection(ctx, c.service.opts.Budget, "snapshot-policy-control", func(raw []byte) ([]byte, error) {
			if len(raw) > 0 {
				var old snapshotPolicyProjection
				if json.Unmarshal(raw, &old) != nil || old.Version != 1 || old.Scope != p.Scope || old.Revision > p.Revision || old.Revision == p.Revision && old.Digest != p.Digest {
					return nil, ErrStateUnavailable
				}
			}
			return json.Marshal(snapshotPolicyProjection{Version: 1, Scope: p.Scope, Revision: p.Revision, Digest: p.Digest, Document: p.Document, ActorID: p.ActorID, UpdatedAt: p.UpdatedAt})
		})
		return err
	})
}
func (c *snapshotPolicyControl) load(ctx context.Context) (snapshot.PolicyConfiguration, ControlState, snapshotPolicyDocument, snapshotPolicyDocument, error) {
	out := snapshot.PolicyConfiguration{Authority: "unavailable", State: "unavailable", Policies: []snapshot.PolicyEntry{}, ExecutionReady: map[string]bool{}}
	var state ControlState
	var desired, applied snapshotPolicyDocument
	fail := func(code string) (snapshot.PolicyConfiguration, ControlState, snapshotPolicyDocument, snapshotPolicyDocument, error) {
		out.Authority = "unavailable"
		out.Ready = false
		out.State = "unconfirmed"
		out.Problem = code
		return out, state, desired, applied, controlError(503, code)
	}
	ctrl := c.service.opts.Control
	if !ctrl.Ready(ctx) {
		return fail("STORAGE_CONTROL_SCHEMA_NOT_READY")
	}
	var raw []byte
	var binding snapshot.PolicyBinding
	e := ctrl.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return ctrl.repo.WithCurrentTx(ctx, tx, ctrl.opts.Scope.ID(), string(ControlSnapshotPolicies), func(row *m.ControlConfiguration) error {
			state = ControlState{Scope: ctrl.opts.Scope, Domain: ControlSnapshotPolicies, State: "unconfigured"}
			if row != nil {
				if row.HostID != ctrl.opts.Scope.HostID || row.FilesystemID != ctrl.opts.Scope.FilesystemID || row.InstanceID != ctrl.opts.Scope.InstanceID {
					return ErrStateUnavailable
				}
				if _, e := verifiedControlDocument(row.Document, row.Digest); e != nil {
					return e
				}
				state = ctrl.state(row)
			}
			var err error
			binding, err = c.snapshots.ReadPolicyBinding()
			if err != nil || binding.ScopeID != state.Scope.ID() {
				return ErrStateUnavailable
			}
			raw, err = readArchiveControlProjection(ctx, c.service.opts.Budget, "snapshot-policy-control")
			return err
		})
	})
	if e != nil {
		return fail("SNAPSHOT_POLICY_AUTHORITY_UNAVAILABLE")
	}
	out.Ready = c.service.opts.ControlWorkerEnabled && !ctrl.ConsumerExited()
	if state.State == "unconfigured" {
		if len(raw) > 0 || binding.Revision > 0 {
			return fail("SNAPSHOT_POLICY_RESTORE_CONFIRMATION_REQUIRED")
		}
		legacy, hash, err := c.snapshots.LegacyPolicySnapshot(ctx)
		if err != nil {
			return fail("SNAPSHOT_POLICY_LEGACY_UNAVAILABLE")
		}
		out.Authority = "legacy"
		out.State = "legacy"
		out.LegacyDigest = hash
		out.Policies = legacy.Policies
		return out, state, desired, applied, nil
	}
	desired, e = decodeSnapshotPolicies(state.Document)
	if e != nil {
		return fail("SNAPSHOT_POLICY_DOCUMENT_INVALID")
	}
	if binding.Revision > state.DesiredRevision || binding.Revision > 0 && binding.OriginDigest != desired.OriginDigest {
		return fail("SNAPSHOT_POLICY_RESTORE_CONFIRMATION_REQUIRED")
	}
	out.Authority = "database"
	out.State = state.State
	out.DesiredRevision = state.DesiredRevision
	out.AppliedRevision = state.AppliedRevision
	out.Policies = desired.Policies
	out.LegacyDigest = desired.OriginDigest
	out.Problem = state.Problem
	if len(raw) == 0 && state.AppliedRevision == 0 {
		return out, state, desired, applied, nil
	}
	var p snapshotPolicyProjection
	if json.Unmarshal(raw, &p) != nil || p.Version != 1 || p.Scope != state.Scope || p.Revision != state.AppliedRevision || p.Revision < 1 || binding.Revision != p.Revision {
		return fail("SNAPSHOT_POLICY_SNAPSHOT_UNCONFIRMED")
	}
	out.SnapshotRevision = &p.Revision
	v, e := ctrl.repo.Revision(ctx, state.Scope.ID(), string(ControlSnapshotPolicies), p.Revision)
	if e != nil || v.Digest != p.Digest || v.ActorID != p.ActorID || !v.CreatedAt.Equal(p.UpdatedAt) {
		return fail("SNAPSHOT_POLICY_SNAPSHOT_UNCONFIRMED")
	}
	if _, e = verifiedControlDocument(v.Document, v.Digest); e != nil {
		return fail("SNAPSHOT_POLICY_SNAPSHOT_UNCONFIRMED")
	}
	if _, e = verifiedControlDocument(p.Document, p.Digest); e != nil {
		return fail("SNAPSHOT_POLICY_SNAPSHOT_UNCONFIRMED")
	}
	applied, e = decodeSnapshotPolicies(p.Document)
	if e != nil {
		return fail("SNAPSHOT_POLICY_DOCUMENT_INVALID")
	}
	for _, want := range desired.Policies {
		for _, active := range applied.Policies {
			if want == active && want.Fingerprint == c.snapshots.PolicyFingerprint() {
				out.ExecutionReady[want.ID] = true
			}
		}
	}
	return out, state, desired, applied, nil
}
func (c *snapshotPolicyControl) Configuration(ctx context.Context) (snapshot.PolicyConfiguration, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return snapshot.PolicyConfiguration{}, ErrRootRequired
	}
	out, _, _, _, e := c.load(ctx)
	return out, e
}
func (c *snapshotPolicyControl) writable(ctx context.Context) (snapshot.PolicyConfiguration, snapshotPolicyDocument, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return snapshot.PolicyConfiguration{}, snapshotPolicyDocument{}, ErrRootRequired
	}
	out, _, desired, _, e := c.load(ctx)
	if e == nil && !out.Ready {
		e = controlError(503, "STORAGE_CONTROL_WORKER_UNAVAILABLE")
	}
	return out, desired, e
}
func (c *snapshotPolicyControl) Adopt(ctx context.Context, in snapshot.PolicyAdoption) (snapshot.PolicyMutation, error) {
	if !in.Confirm || !validTargetUUID(in.RequestID) {
		return snapshot.PolicyMutation{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	op := snapshotPolicyOperation{Action: "adopt", Adoption: &in}
	if out, ok, e := c.replay(ctx, in.RequestID, op); ok {
		return out, e
	}
	conf, _, e := c.writable(ctx)
	if e != nil {
		return snapshot.PolicyMutation{}, e
	}
	if conf.Authority != "legacy" || conf.LegacyDigest != in.LegacyDigest {
		return snapshot.PolicyMutation{}, controlError(409, "SNAPSHOT_POLICY_LEGACY_CHANGED")
	}
	origin, hash, e := c.snapshots.LegacyPolicySnapshot(ctx)
	if e != nil {
		return snapshot.PolicyMutation{}, e
	}
	if hash != in.LegacyDigest {
		return snapshot.PolicyMutation{}, controlError(409, "SNAPSHOT_POLICY_LEGACY_CHANGED")
	}
	policies := append([]snapshot.PolicyEntry{}, origin.Policies...)
	for i := range policies {
		policies[i].Revision = 1
		policies[i].Enabled = false
		policies[i].Current = false
		policies[i].Anchor = time.Time{}
		policies[i].Fingerprint = c.snapshots.PolicyFingerprint()
	}
	return c.save(ctx, 0, in.RequestID, snapshotPolicyDocument{Version: 1, Origin: origin, OriginDigest: hash, Policies: policies, Operation: op})
}
func (c *snapshotPolicyControl) Change(ctx context.Context, in snapshot.PolicyChange) (snapshot.PolicyMutation, error) {
	if !in.Confirm || !validTargetUUID(in.RequestID) || in.ExpectedRevision < 1 {
		return snapshot.PolicyMutation{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	if in.Action != "save" && in.Action != "status" && in.Action != "current" {
		return snapshot.PolicyMutation{}, controlError(400, "SNAPSHOT_POLICY_ACTION_INVALID")
	}
	if in.Action == "save" {
		if in.Settings == nil || in.Enabled != nil || snapshot.ValidatePolicySettings(*in.Settings) != nil {
			return snapshot.PolicyMutation{}, controlError(400, "SNAPSHOT_POLICY_SETTINGS_INVALID")
		}
	} else if in.Settings != nil || in.Action == "status" && in.Enabled == nil || in.Action == "current" && in.Enabled != nil {
		return snapshot.PolicyMutation{}, controlError(400, "SNAPSHOT_POLICY_ACTION_INVALID")
	}
	op := snapshotPolicyOperation{Action: "change", Change: &in}
	if out, ok, e := c.replay(ctx, in.RequestID, op); ok {
		return out, e
	}
	conf, d, e := c.writable(ctx)
	if e != nil {
		return snapshot.PolicyMutation{}, e
	}
	if conf.Authority != "database" {
		return snapshot.PolicyMutation{}, controlError(428, "SNAPSHOT_POLICY_ADOPTION_REQUIRED")
	}
	if conf.DesiredRevision != in.ExpectedRevision {
		return snapshot.PolicyMutation{}, controlError(409, "STORAGE_CONTROL_REVISION_CONFLICT")
	}
	id := in.PolicyID
	if id == "" {
		id = in.RequestID
	}
	index := -1
	for i, p := range d.Policies {
		if p.ID == id {
			index = i
		}
	}
	if index < 0 {
		if in.Action != "save" || in.PolicyID != "" || in.ExpectedPolicyRevision != 0 {
			return snapshot.PolicyMutation{}, controlError(404, "SNAPSHOT_POLICY_NOT_FOUND")
		}
		d.Policies = append(d.Policies, snapshot.PolicyEntry{ID: id})
		index = len(d.Policies) - 1
	}
	p := &d.Policies[index]
	if p.Revision != in.ExpectedPolicyRevision {
		return snapshot.PolicyMutation{}, controlError(409, "SNAPSHOT_POLICY_REVISION_CONFLICT")
	}
	p.Revision++
	switch in.Action {
	case "save":
		p.Settings = *in.Settings
		p.Enabled = false
		p.Current = false
		p.Anchor = time.Time{}
		p.Fingerprint = c.snapshots.PolicyFingerprint()
	case "status":
		if *in.Enabled && (!p.Current || p.Fingerprint != c.snapshots.PolicyFingerprint()) {
			return snapshot.PolicyMutation{}, controlError(409, "SNAPSHOT_POLICY_CURRENT_SOURCE_REQUIRED")
		}
		if *in.Enabled {
			if e := c.snapshots.CheckPolicySources(ctx); e != nil {
				return snapshot.PolicyMutation{}, e
			}
		}
		p.Enabled = *in.Enabled
		if p.Enabled {
			p.Anchor = time.Now().UTC()
		}
	case "current":
		if p.Fingerprint != c.snapshots.PolicyFingerprint() {
			return snapshot.PolicyMutation{}, controlError(409, "SNAPSHOT_POLICY_SOURCE_CHANGED")
		}
		for i := range d.Policies {
			if i != index && d.Policies[i].Current {
				d.Policies[i].Current = false
				d.Policies[i].Enabled = false
				d.Policies[i].Revision++
			}
		}
		p.Current = true
		p.Enabled = false
		p.Anchor = time.Time{}
	}
	d.Operation = op
	sort.Slice(d.Policies, func(i, j int) bool { return d.Policies[i].ID < d.Policies[j].ID })
	return c.save(ctx, in.ExpectedRevision, in.RequestID, d)
}
func (c *snapshotPolicyControl) save(ctx context.Context, revision int64, key string, d snapshotPolicyDocument) (snapshot.PolicyMutation, error) {
	raw, e := json.Marshal(d)
	if e != nil {
		return snapshot.PolicyMutation{}, e
	}
	_, e = c.service.opts.Control.Save(ctx, ControlInput{RequestID: key, ExpectedRevision: revision, Domain: ControlSnapshotPolicies, Document: raw})
	if e != nil {
		return snapshot.PolicyMutation{}, e
	}
	return c.Request(ctx, key)
}
func (c *snapshotPolicyControl) replay(ctx context.Context, key string, op snapshotPolicyOperation) (snapshot.PolicyMutation, bool, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return snapshot.PolicyMutation{}, true, ErrRootRequired
	}
	v, e := c.service.opts.Control.repo.Request(ctx, c.service.opts.Control.opts.Scope.ID(), key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return snapshot.PolicyMutation{}, false, nil
	}
	if e != nil {
		return snapshot.PolicyMutation{}, true, ErrStateUnavailable
	}
	if v.Domain != string(ControlSnapshotPolicies) || v.ActorID != reqctx.GetUserID(ctx) {
		return snapshot.PolicyMutation{}, true, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
	}
	d, e := decodeSnapshotPolicies(v.Document)
	if e != nil {
		return snapshot.PolicyMutation{}, true, e
	}
	a, _ := json.Marshal(op)
	b, _ := json.Marshal(d.Operation)
	if !bytes.Equal(a, b) {
		return snapshot.PolicyMutation{}, true, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
	}
	out, e := c.Request(ctx, key)
	return out, true, e
}
func (c *snapshotPolicyControl) Request(ctx context.Context, key string) (snapshot.PolicyMutation, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return snapshot.PolicyMutation{}, ErrRootRequired
	}
	if !validTargetUUID(key) {
		return snapshot.PolicyMutation{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	v, e := c.service.opts.Control.repo.Request(ctx, c.service.opts.Control.opts.Scope.ID(), key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return snapshot.PolicyMutation{}, controlError(404, "STORAGE_CONTROL_REQUEST_NOT_FOUND")
	}
	if e != nil {
		return snapshot.PolicyMutation{}, ErrStateUnavailable
	}
	if v.Domain != string(ControlSnapshotPolicies) || v.ActorID != reqctx.GetUserID(ctx) {
		return snapshot.PolicyMutation{}, controlError(403, "STORAGE_CONTROL_REQUEST_FORBIDDEN")
	}
	if _, e = verifiedControlDocument(v.Document, v.Digest); e != nil {
		return snapshot.PolicyMutation{}, e
	}
	d, e := decodeSnapshotPolicies(v.Document)
	if e != nil {
		return snapshot.PolicyMutation{}, e
	}
	id := ""
	if d.Operation.Change != nil {
		id = d.Operation.Change.PolicyID
		if id == "" {
			id = key
		}
	}
	conf, _, _, _, _ := c.load(ctx)
	return snapshot.PolicyMutation{RequestID: key, OperationRevision: v.Revision, PolicyID: id, Configuration: conf}, nil
}
func (c *snapshotPolicyControl) Runtime(ctx context.Context) ([]snapshot.PolicyEntry, error) {
	conf, _, _, _, e := c.load(ctx)
	if e != nil {
		return nil, e
	}
	if conf.Authority != "database" {
		return nil, controlError(428, "SNAPSHOT_POLICY_ADOPTION_REQUIRED")
	}
	entries := append([]snapshot.PolicyEntry{}, conf.Policies...)
	for i := range entries {
		if !conf.ExecutionReady[entries[i].ID] {
			entries[i].Current = false
			entries[i].Enabled = false
		}
	}
	return entries, nil
}
func (c *snapshotPolicyControl) Guard(ctx context.Context, id string, revision int64, operation string, action func(snapshot.PolicyEntry) error) error {
	if operation != "manual" && operation != "scheduled" && operation != "cleanup" {
		return controlError(400, "SNAPSHOT_POLICY_ACTION_INVALID")
	}
	run, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conf, state, desired, applied, e := c.load(run)
	if e != nil {
		return e
	}
	if conf.Authority != "database" {
		return controlError(428, "SNAPSHOT_POLICY_ADOPTION_REQUIRED")
	}
	return c.service.opts.DB.WithContext(run).Transaction(func(tx *gorm.DB) error {
		return c.service.opts.Control.WithExecutionTx(run, tx, ControlSnapshotPolicies, func(row *m.ControlConfiguration) error {
			if row == nil || row.Revision != state.DesiredRevision || row.AppliedRevision != state.AppliedRevision || row.Digest != state.Digest {
				return controlError(409, "SNAPSHOT_POLICY_CHANGED")
			}
			var want, active *snapshot.PolicyEntry
			for i := range desired.Policies {
				if desired.Policies[i].ID == id {
					want = &desired.Policies[i]
				}
			}
			for i := range applied.Policies {
				if applied.Policies[i].ID == id {
					active = &applied.Policies[i]
				}
			}
			if want == nil || active == nil || *want != *active || revision > 0 && want.Revision != revision {
				return controlError(409, "SNAPSHOT_POLICY_PUBLICATION_PENDING")
			}
			if want.Fingerprint != c.snapshots.PolicyFingerprint() {
				return controlError(409, "SNAPSHOT_POLICY_SOURCE_CHANGED")
			}
			if !want.Current || (operation != "manual" && !want.Enabled) {
				return controlError(409, "SNAPSHOT_POLICY_DISABLED")
			}
			if run.Err() != nil {
				return run.Err()
			}
			return action(*want)
		})
	})
}
