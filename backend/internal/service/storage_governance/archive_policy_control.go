package storagegov

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"time"

	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
)

type archivePolicyEntry struct {
	ID                   string                 `json:"id"`
	Revision             int64                  `json:"revision"`
	Settings             archive.PolicySettings `json:"settings"`
	Enabled              bool                   `json:"enabled"`
	RemoteCleanupEnabled bool                   `json:"remote_cleanup_enabled"`
	SourceFingerprint    string                 `json:"source_fingerprint"`
	TargetRevision       int64                  `json:"target_revision"`
	TargetFingerprint    string                 `json:"target_fingerprint"`
	CreatedAt            time.Time              `json:"created_at"`
}
type archivePolicyOperation struct {
	Action   string                  `json:"action"`
	Change   *archive.PolicyChange   `json:"change,omitempty"`
	Adoption *archive.PolicyAdoption `json:"adoption,omitempty"`
}
type archivePolicyDocument struct {
	Recovery     *RecoveryStamp         `json:"recovery,omitempty"`
	Version      int                    `json:"version"`
	Entries      []archivePolicyEntry   `json:"entries"`
	OriginDigest string                 `json:"origin_digest"`
	Origin       []archive.Policy       `json:"origin"`
	Operation    archivePolicyOperation `json:"operation"`
}
type archivePolicyProjection struct {
	Version   int             `json:"version"`
	Scope     ControlScope    `json:"scope"`
	Revision  int64           `json:"revision"`
	Digest    string          `json:"digest"`
	Document  json.RawMessage `json:"document"`
	ActorID   uint64          `json:"actor_id"`
	UpdatedAt time.Time       `json:"updated_at"`
}
type archivePolicyControl struct {
	service  *Service
	archives *archive.Service
}

// bindArchivePolicyControl 绑定固定策略域，不自动导入或恢复历史启用值。
func (s *Service) bindArchivePolicyControl(a *archive.Service) error {
	if s.opts.Control == nil || s.opts.Budget == nil {
		return controlError(503, "ARCHIVE_POLICY_CONTROL_UNAVAILABLE")
	}
	c := &archivePolicyControl{s, a}
	if e := s.opts.Control.Bind(ControlArchivePolicies, ControlBinding{Validate: func(raw json.RawMessage) error { _, e := decodeArchivePolicies(raw); return e }, Apply: c.publish}); e != nil {
		return e
	}
	return a.BindPolicyAuthority(c)
}
func decodeArchivePolicies(raw []byte) (archivePolicyDocument, error) {
	var doc archivePolicyDocument
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&doc) != nil || d.Decode(new(any)) != io.EOF || doc.Version != 1 || len(doc.Entries) > 128 || len(doc.Origin) > 128 || len(doc.OriginDigest) != 64 {
		return doc, controlError(400, "ARCHIVE_POLICY_DOCUMENT_INVALID")
	}
	seen := map[string]bool{}
	for _, entry := range doc.Entries {
		if !validTargetUUID(entry.ID) || seen[entry.ID] || entry.Revision < 1 {
			return doc, controlError(400, "ARCHIVE_POLICY_DOCUMENT_INVALID")
		}
		seen[entry.ID] = true
		if len(entry.SourceFingerprint) != 64 {
			return doc, controlError(400, "ARCHIVE_POLICY_DOCUMENT_INVALID")
		}
		if e := archive.ValidatePolicySettings(entry.Settings); e != nil {
			return doc, e
		}
	}
	if (doc.Operation.Action != "adopt" || doc.Operation.Adoption == nil || doc.Operation.Change != nil) && (doc.Operation.Action != "save" || doc.Operation.Change == nil || doc.Operation.Adoption != nil) {
		return doc, controlError(400, "ARCHIVE_POLICY_DOCUMENT_INVALID")
	}
	return doc, nil
}
func (c *archivePolicyControl) legacy(ctx context.Context) ([]archive.Policy, string, error) {
	policies, e := c.archives.LegacyPolicies(ctx)
	if e != nil {
		return nil, "", e
	}
	raw, e := json.Marshal(struct {
		Policies []archive.Policy `json:"policies"`
	}{policies})
	if e == nil {
		raw, e = canonicalControl(raw)
	}
	return policies, controlDigest(raw), e
}
func (c *archivePolicyControl) publish(ctx context.Context, p ControlPublication) error {
	if p.Scope != c.service.opts.Control.opts.Scope {
		return ErrStateUnavailable
	}
	doc, e := decodeArchivePolicies(p.Document)
	if e != nil {
		return e
	}
	if p.Revision == 1 {
		_, digest, e := c.legacy(ctx)
		if e != nil {
			return e
		}
		if digest != doc.OriginDigest {
			return controlError(409, "ARCHIVE_POLICY_LEGACY_CHANGED")
		}
	}
	_, e = updateArchiveControlProjection(ctx, c.service.opts.Budget, "archive-policy-control", func(raw []byte) ([]byte, error) {
		// 首次投影可能已被更高期望版本取代，仍须核对接管来源，不能只检查 revision=1。
		if len(raw) == 0 {
			_, digest, err := c.legacy(ctx)
			if err != nil {
				return nil, err
			}
			if digest != doc.OriginDigest {
				return nil, controlError(409, "ARCHIVE_POLICY_LEGACY_CHANGED")
			}
		}
		if len(raw) > 0 {
			var old archivePolicyProjection
			if json.Unmarshal(raw, &old) != nil || old.Version != 1 || old.Scope != p.Scope || old.Revision > p.Revision || old.Revision == p.Revision && old.Digest != p.Digest {
				return nil, ErrStateUnavailable
			}
		}
		return json.Marshal(archivePolicyProjection{1, p.Scope, p.Revision, p.Digest, p.Document, p.ActorID, p.UpdatedAt})
	})
	return e
}
func policyEntry(doc archivePolicyDocument, id string) (archivePolicyEntry, bool) {
	for _, e := range doc.Entries {
		if e.ID == id {
			return e, true
		}
	}
	return archivePolicyEntry{}, false
}
func effectiveArchivePolicy(entry archivePolicyEntry, scope ControlScope) archive.Policy {
	t := entry.Settings.Policy(entry.ID, entry.Revision)
	t.Enabled = entry.Enabled
	t.RemoteCleanupEnabled = entry.RemoteCleanupEnabled
	t.TargetRevision = entry.TargetRevision
	t.TargetFingerprint = entry.TargetFingerprint
	t.CreatedAt = entry.CreatedAt
	raw, _ := json.Marshal(struct {
		Scope ControlScope
		Entry archivePolicyEntry
	}{scope, entry})
	t.ControlDigest = controlDigest(raw)
	return t
}
func (c *archivePolicyControl) load(ctx context.Context) (archive.PolicyConfiguration, archivePolicyDocument, archivePolicyDocument, error) {
	out := archive.PolicyConfiguration{Authority: "unavailable", State: "unavailable", Policies: []archive.Policy{}}
	var desired, applied archivePolicyDocument
	fail := func(code string) (archive.PolicyConfiguration, archivePolicyDocument, archivePolicyDocument, error) {
		out.Problem = code
		out.State = "unconfirmed"
		return out, desired, archivePolicyDocument{}, controlError(503, code)
	}
	ctrl := c.service.opts.Control
	if !ctrl.Ready(ctx) {
		return fail("STORAGE_CONTROL_SCHEMA_NOT_READY")
	}
	state, e := ctrl.read(ctx, ControlArchivePolicies)
	if e != nil {
		return fail("STORAGE_CONTROL_UNAVAILABLE")
	}
	out.Ready = c.service.opts.ControlWorkerEnabled && !ctrl.ConsumerExited()
	raw, e := readArchiveControlProjection(ctx, c.service.opts.Budget, "archive-policy-control")
	if e != nil {
		return fail("ARCHIVE_POLICY_SNAPSHOT_UNAVAILABLE")
	}
	if state.State == "unconfigured" {
		if len(raw) > 0 {
			return fail("ARCHIVE_POLICY_RESTORE_CONFIRMATION_REQUIRED")
		}
		policies, digest, e := c.legacy(ctx)
		if e != nil {
			return fail("ARCHIVE_POLICY_LEGACY_UNAVAILABLE")
		}
		out.Authority, out.State, out.LegacyDigest, out.LegacyCount = "legacy", "legacy", digest, len(policies)
		for _, t := range policies {
			t.Enabled = false
			t.RemoteCleanupEnabled = false
			t.TargetRevision = 0
			t.TargetFingerprint = ""
			out.Policies = append(out.Policies, t)
		}
		return out, desired, applied, nil
	}
	desired, e = decodeArchivePolicies(state.Document)
	if e != nil {
		return fail("ARCHIVE_POLICY_DOCUMENT_INVALID")
	}
	out.Authority, out.State, out.DesiredRevision, out.AppliedRevision, out.Problem = "database", state.State, state.DesiredRevision, state.AppliedRevision, state.Problem
	out.LegacyDigest, out.LegacyCount = desired.OriginDigest, len(desired.Origin)
	for _, entry := range desired.Entries {
		out.Policies = append(out.Policies, effectiveArchivePolicy(entry, state.Scope))
	}
	if ctrl.ConsumerExited() {
		out.Problem = "STORAGE_CONTROL_WORKER_UNAVAILABLE"
	}
	if len(raw) == 0 && state.AppliedRevision == 0 {
		return out, desired, applied, nil
	}
	var p archivePolicyProjection
	if json.Unmarshal(raw, &p) != nil || p.Version != 1 || p.Scope != state.Scope || p.Revision != state.AppliedRevision || p.Revision < 1 {
		return fail("ARCHIVE_POLICY_SNAPSHOT_UNCONFIRMED")
	}
	out.SnapshotRevision = &p.Revision
	version, e := ctrl.repo.Revision(ctx, state.Scope.ID(), string(ControlArchivePolicies), p.Revision)
	if e != nil || version.Digest != p.Digest || version.ActorID != p.ActorID || !version.CreatedAt.Equal(p.UpdatedAt) {
		return fail("ARCHIVE_POLICY_SNAPSHOT_UNCONFIRMED")
	}
	if _, e = verifiedControlDocument(p.Document, version.Digest); e != nil {
		return fail("ARCHIVE_POLICY_SNAPSHOT_UNCONFIRMED")
	}
	if _, e = verifiedControlDocument(version.Document, version.Digest); e != nil {
		return fail("ARCHIVE_POLICY_SNAPSHOT_UNCONFIRMED")
	}
	applied, e = decodeArchivePolicies(p.Document)
	if e != nil {
		return fail("ARCHIVE_POLICY_DOCUMENT_INVALID")
	}
	return out, desired, applied, nil
}
func (c *archivePolicyControl) Configuration(ctx context.Context) (archive.PolicyConfiguration, error) {
	out, _, _, e := c.load(ctx)
	return out, e
}
func (c *archivePolicyControl) Resolve(ctx context.Context, id string) (archive.Policy, error) {
	out, desired, applied, e := c.load(ctx)
	if e != nil {
		return archive.Policy{}, e
	}
	if out.Authority != "database" {
		return archive.Policy{}, controlError(428, "ARCHIVE_POLICY_ADOPTION_REQUIRED")
	}
	want, ok := policyEntry(desired, id)
	if !ok {
		return archive.Policy{}, controlError(404, "ARCHIVE_POLICY_NOT_FOUND")
	}
	active, ok := policyEntry(applied, id)
	if !ok || active != want {
		return archive.Policy{}, controlError(409, "ARCHIVE_POLICY_PUBLICATION_PENDING")
	}
	p := effectiveArchivePolicy(active, c.service.opts.Control.opts.Scope)
	if e := c.archives.CheckPolicyExecution(ctx, p, active.SourceFingerprint); e != nil {
		return archive.Policy{}, e
	}
	return p, nil
}
func (c *archivePolicyControl) requireWrite(ctx context.Context) error {
	if !auth.IsPlatformRootSession(ctx) {
		return ErrRootRequired
	}
	if !c.service.opts.ControlWorkerEnabled || !c.service.opts.Control.Ready(ctx) || c.service.opts.Control.ConsumerExited() {
		return controlError(503, "ARCHIVE_POLICY_CONTROL_UNAVAILABLE")
	}
	return nil
}
func (c *archivePolicyControl) replay(ctx context.Context, key string, op archivePolicyOperation) (archive.PolicyMutation, bool, error) {
	row, e := c.service.opts.Control.repo.Request(ctx, c.service.opts.Control.opts.Scope.ID(), key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return archive.PolicyMutation{}, false, nil
	}
	if e != nil {
		return archive.PolicyMutation{}, true, controlError(503, "STORAGE_CONTROL_UNAVAILABLE")
	}
	if row.Domain != string(ControlArchivePolicies) || row.ActorID != reqctx.GetClaims(ctx).UserID {
		return archive.PolicyMutation{}, true, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
	}
	if _, e = verifiedControlDocument(row.Document, row.Digest); e != nil {
		return archive.PolicyMutation{}, true, e
	}
	doc, e := decodeArchivePolicies(row.Document)
	if e != nil {
		return archive.PolicyMutation{}, true, e
	}
	a, _ := json.Marshal(doc.Operation)
	b, _ := json.Marshal(op)
	if !bytes.Equal(a, b) {
		return archive.PolicyMutation{}, true, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
	}
	out, e := c.commit(ctx, row.Revision-1, key, doc)
	return out, true, e
}
func (c *archivePolicyControl) Adopt(ctx context.Context, in archive.PolicyAdoption) (archive.PolicyMutation, error) {
	if e := c.requireWrite(ctx); e != nil {
		return archive.PolicyMutation{}, e
	}
	if !validTargetUUID(in.RequestID) {
		return archive.PolicyMutation{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	op := archivePolicyOperation{Action: "adopt", Adoption: &in}
	if out, found, e := c.replay(ctx, in.RequestID, op); found {
		return out, e
	}
	origin, digest, e := c.legacy(ctx)
	if e != nil {
		return archive.PolicyMutation{}, e
	}
	if in.LegacyDigest != digest {
		return archive.PolicyMutation{}, controlError(409, "ARCHIVE_POLICY_LEGACY_CHANGED")
	}
	doc := archivePolicyDocument{Version: 1, Entries: []archivePolicyEntry{}, OriginDigest: digest, Origin: origin, Operation: op}
	for _, t := range origin {
		fingerprint, e := c.archives.PolicySourceFingerprint(t.SourceID)
		if e != nil {
			return archive.PolicyMutation{}, e
		}
		doc.Entries = append(doc.Entries, archivePolicyEntry{ID: t.ID, Revision: t.Revision + 1, Settings: archive.SettingsOfPolicy(t), SourceFingerprint: fingerprint, CreatedAt: t.CreatedAt})
	}
	return c.commit(ctx, 0, in.RequestID, doc)
}
func (c *archivePolicyControl) Save(ctx context.Context, in archive.PolicyChange) (archive.PolicyMutation, error) {
	if e := c.requireWrite(ctx); e != nil {
		return archive.PolicyMutation{}, e
	}
	if !validTargetUUID(in.RequestID) || in.ExpectedRevision < 1 || in.ExpectedPolicyRevision < 0 || in.PolicyID != "" && !validTargetUUID(in.PolicyID) {
		return archive.PolicyMutation{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	if in.Action == "save" {
		if in.Settings == nil || in.Enabled != nil || in.RemoteCleanupEnabled != nil || in.Confirm || in.ExpectedTargetRevision != 0 || in.ExpectedTargetFingerprint != "" {
			return archive.PolicyMutation{}, controlError(400, "ARCHIVE_POLICY_ACTION_INVALID")
		}
		if e := archive.ValidatePolicySettings(*in.Settings); e != nil {
			return archive.PolicyMutation{}, e
		}
	} else if in.Action == "status" {
		if in.PolicyID == "" || in.Settings != nil || !in.Confirm || in.Enabled == nil || in.RemoteCleanupEnabled == nil {
			return archive.PolicyMutation{}, controlError(400, "ARCHIVE_POLICY_CONFIRMATION_REQUIRED")
		}
	} else {
		return archive.PolicyMutation{}, controlError(400, "ARCHIVE_POLICY_ACTION_INVALID")
	}
	op := archivePolicyOperation{Action: "save", Change: &in}
	if out, found, e := c.replay(ctx, in.RequestID, op); found {
		return out, e
	}
	state, e := c.service.opts.Control.read(ctx, ControlArchivePolicies)
	if e != nil {
		return archive.PolicyMutation{}, e
	}
	if state.State == "unconfigured" {
		return archive.PolicyMutation{}, controlError(428, "ARCHIVE_POLICY_ADOPTION_REQUIRED")
	}
	if state.DesiredRevision != in.ExpectedRevision {
		return archive.PolicyMutation{}, controlError(409, "STORAGE_CONTROL_REVISION_CONFLICT")
	}
	doc, e := decodeArchivePolicies(state.Document)
	if e != nil {
		return archive.PolicyMutation{}, e
	}
	id := in.PolicyID
	if id == "" {
		id = in.RequestID
	}
	old, exists := policyEntry(doc, id)
	if exists && (in.PolicyID == "" || old.Revision != in.ExpectedPolicyRevision) || !exists && (in.PolicyID != "" || in.ExpectedPolicyRevision != 0) {
		return archive.PolicyMutation{}, controlError(409, "ARCHIVE_REVISION_CONFLICT")
	}
	entry := old
	entry.ID = id
	entry.Revision = in.ExpectedPolicyRevision + 1
	if in.Action == "save" {
		entry.Settings = *in.Settings
		entry.Enabled = false
		entry.RemoteCleanupEnabled = false
		entry.TargetRevision = 0
		entry.TargetFingerprint = ""
		if _, e = c.archives.PreparePolicy(ctx, entry.Settings.Policy(id, entry.Revision)); e != nil {
			return archive.PolicyMutation{}, e
		}
		entry.SourceFingerprint, e = c.archives.PolicySourceFingerprint(entry.Settings.SourceID)
		if e != nil {
			return archive.PolicyMutation{}, e
		}
	} else {
		entry.Enabled = *in.Enabled
		entry.RemoteCleanupEnabled = *in.RemoteCleanupEnabled
		if entry.Enabled || entry.RemoteCleanupEnabled {
			fp, e := c.archives.PolicySourceFingerprint(entry.Settings.SourceID)
			if e != nil || fp != entry.SourceFingerprint {
				return archive.PolicyMutation{}, controlError(409, "ARCHIVE_POLICY_SOURCE_CHANGED")
			}
			p := effectiveArchivePolicy(entry, state.Scope)
			p, e = c.archives.PreparePolicy(ctx, p)
			if e != nil {
				return archive.PolicyMutation{}, e
			}
			if p.TargetRevision != in.ExpectedTargetRevision || p.TargetFingerprint != in.ExpectedTargetFingerprint {
				return archive.PolicyMutation{}, controlError(409, "ARCHIVE_POLICY_TARGET_CHANGED")
			}
			entry.TargetRevision = p.TargetRevision
			entry.TargetFingerprint = p.TargetFingerprint
		}
	}
	if !exists {
		entry.CreatedAt = time.Now().UTC()
		doc.Entries = append(doc.Entries, entry)
	} else {
		for i := range doc.Entries {
			if doc.Entries[i].ID == id {
				doc.Entries[i] = entry
			}
		}
	}
	sort.Slice(doc.Entries, func(i, j int) bool { return doc.Entries[i].ID < doc.Entries[j].ID })
	doc.Operation = op
	return c.commit(ctx, in.ExpectedRevision, in.RequestID, doc)
}
func (c *archivePolicyControl) commit(ctx context.Context, revision int64, key string, doc archivePolicyDocument) (archive.PolicyMutation, error) {
	raw, e := json.Marshal(doc)
	if e != nil {
		return archive.PolicyMutation{}, e
	}
	state, e := c.service.opts.Control.Save(ctx, ControlInput{key, revision, ControlArchivePolicies, raw})
	if e != nil {
		return archive.PolicyMutation{}, e
	}
	out, _, _, _ := c.load(ctx)
	id := ""
	if doc.Operation.Change != nil {
		id = doc.Operation.Change.PolicyID
		if id == "" {
			id = key
		}
	}
	return archive.PolicyMutation{RequestID: key, OperationRevision: state.OperationRevision, PolicyID: id, Configuration: out}, nil
}
func (c *archivePolicyControl) Request(ctx context.Context, key string) (archive.PolicyMutation, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return archive.PolicyMutation{}, ErrRootRequired
	}
	if !validTargetUUID(key) {
		return archive.PolicyMutation{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	row, e := c.service.opts.Control.repo.Request(ctx, c.service.opts.Control.opts.Scope.ID(), key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return archive.PolicyMutation{}, controlError(404, "STORAGE_CONTROL_REQUEST_NOT_FOUND")
	}
	if e != nil {
		return archive.PolicyMutation{}, controlError(503, "STORAGE_CONTROL_UNAVAILABLE")
	}
	if row.Domain != string(ControlArchivePolicies) || row.ActorID != reqctx.GetClaims(ctx).UserID {
		return archive.PolicyMutation{}, controlError(403, "STORAGE_CONTROL_REQUEST_FORBIDDEN")
	}
	if _, e = verifiedControlDocument(row.Document, row.Digest); e != nil {
		return archive.PolicyMutation{}, e
	}
	doc, e := decodeArchivePolicies(row.Document)
	if e != nil {
		return archive.PolicyMutation{}, e
	}
	out, _, _, _ := c.load(ctx)
	id := ""
	if doc.Operation.Change != nil {
		id = doc.Operation.Change.PolicyID
		if id == "" {
			id = key
		}
	}
	return archive.PolicyMutation{RequestID: key, OperationRevision: row.Revision, PolicyID: id, Configuration: out}, nil
}

// Guard 使用同一事务按策略→目标固定顺序锁定权威，暂停提交后不会启动下一批写入或删除。
func (c *archivePolicyControl) Guard(ctx context.Context, p archive.Policy, operation string, action func([]archive.Policy, *archive.Target) error) error {
	configuration, desired, applied, e := c.load(ctx)
	if e != nil {
		return e
	}
	if operation != "transfer" && operation != "remote-cleanup" && operation != "local-retention" && operation != "start" && operation != "interruption-retain" {
		return controlError(400, "ARCHIVE_POLICY_ACTION_INVALID")
	}
	if p.ID != "" {
		current, e := c.Resolve(ctx, p.ID)
		if e != nil {
			return e
		}
		if current.ControlDigest != p.ControlDigest {
			return controlError(409, "ARCHIVE_POLICY_CHANGED")
		}
	}
	if p.ID != "" && ((operation == "transfer" || operation == "start") && !p.Enabled || operation == "remote-cleanup" && !p.RemoteCleanupEnabled || operation == "local-retention" && !p.Enabled && !p.RemoteCleanupEnabled) {
		return controlError(409, "ARCHIVE_POLICY_DISABLED")
	}
	if operation == "interruption-retain" && (p.Enabled || p.RemoteCleanupEnabled) {
		return controlError(409, "ARCHIVE_PAUSE_REQUIRED")
	}
	needsTarget := operation == "transfer" || operation == "remote-cleanup" || operation == "start" && p.TargetID != ""
	var target archive.Target
	var targetConfiguration archive.TargetConfiguration
	var targetDesired archiveTargetDocument
	var targetApplied archiveTargetDocument
	if needsTarget {
		target, e = c.archives.PolicyExecutionTarget(ctx, p)
		if e != nil {
			return e
		}
		targetConfiguration, targetDesired, targetApplied, e = (&archiveTargetControl{c.service, c.archives}).load(ctx)
		if e != nil {
			return e
		}
	}
	repo := c.service.opts.Control.repo
	scope := c.service.opts.Control.opts.Scope
	return repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return c.service.opts.Control.WithExecutionTx(ctx, tx, ControlArchivePolicies, func(row *m.ControlConfiguration) error {
			if row == nil || row.AppliedRevision < 1 || row.AppliedRevision != configuration.AppliedRevision {
				return ErrStateUnavailable
			}
			if row.HostID != scope.HostID || row.FilesystemID != scope.FilesystemID || row.InstanceID != scope.InstanceID {
				return ErrStateUnavailable
			}
			if _, e := verifiedControlDocument(row.Document, row.Digest); e != nil {
				return e
			}
			doc, e := decodeArchivePolicies(row.Document)
			if e != nil {
				return e
			}
			before, _ := json.Marshal(desired)
			now, _ := json.Marshal(doc)
			if !bytes.Equal(before, now) {
				return controlError(409, "ARCHIVE_POLICY_CHANGED")
			}
			if p.ID != "" {
				entry, ok := policyEntry(doc, p.ID)
				if !ok || effectiveArchivePolicy(entry, scope).ControlDigest != p.ControlDigest {
					return controlError(409, "ARCHIVE_POLICY_CHANGED")
				}
				if (operation == "transfer" || operation == "start") && !entry.Enabled || operation == "remote-cleanup" && !entry.RemoteCleanupEnabled || operation == "local-retention" && !entry.Enabled && !entry.RemoteCleanupEnabled {
					return controlError(409, "ARCHIVE_POLICY_DISABLED")
				}
			}
			if operation == "interruption-retain" && p.ID != "" {
				entry, ok := policyEntry(doc, p.ID)
				if !ok || entry.Enabled || entry.RemoteCleanupEnabled {
					return controlError(409, "ARCHIVE_PAUSE_REQUIRED")
				}
			}
			frozen := []archive.Policy{}
			for _, entry := range doc.Entries {
				active, ok := policyEntry(applied, entry.ID)
				value := effectiveArchivePolicy(entry, scope)
				if !ok || active != entry {
					value.Enabled = false
					value.RemoteCleanupEnabled = false
				}
				frozen = append(frozen, value)
			}
			if !needsTarget {
				return action(frozen, nil)
			}
			return repo.WithCurrentTx(ctx, tx, scope.ID(), string(ControlArchiveTargets), func(targetRow *m.ControlConfiguration) error {
				if targetRow == nil || targetRow.AppliedRevision != targetConfiguration.AppliedRevision || targetRow.HostID != scope.HostID || targetRow.FilesystemID != scope.FilesystemID || targetRow.InstanceID != scope.InstanceID {
					return ErrStateUnavailable
				}
				if _, e := verifiedControlDocument(targetRow.Document, targetRow.Digest); e != nil {
					return e
				}
				doc, e := decodeArchiveTargets(targetRow.Document)
				if e != nil {
					return e
				}
				before, _ := json.Marshal(targetDesired)
				now, _ := json.Marshal(doc)
				if !bytes.Equal(before, now) {
					return controlError(409, "ARCHIVE_POLICY_TARGET_CHANGED")
				}
				entry, ok := targetEntry(doc, p.TargetID)
				approved, active := targetEntry(targetApplied, p.TargetID)
				if !ok || !active || entry != approved || effectiveArchiveTarget(entry, scope).ControlDigest != target.ControlDigest {
					return controlError(409, "ARCHIVE_POLICY_TARGET_CHANGED")
				}
				if e := c.archives.CheckCapturedTarget(target); e != nil {
					return e
				}
				return action(frozen, &target)
			})
		})
	})
}
func (c *archivePolicyControl) ChangeDirectories() []string {
	return []string{c.service.opts.Budget.ChangeDirectory()}
}
