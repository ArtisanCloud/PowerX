package storagegov

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"time"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type archiveTargetEntry struct {
	ID       string                 `json:"id"`
	Revision int64                  `json:"revision"`
	Settings archive.TargetSettings `json:"settings"`
}
type archiveTargetOperation struct {
	Action   string                  `json:"action"`
	Change   *archive.TargetChange   `json:"change,omitempty"`
	Adoption *archive.TargetAdoption `json:"adoption,omitempty"`
}
type archiveTargetDocument struct {
	Recovery     *RecoveryStamp         `json:"recovery,omitempty"`
	Version      int                    `json:"version"`
	Entries      []archiveTargetEntry   `json:"entries"`
	OriginDigest string                 `json:"origin_digest"`
	Origin       []archive.Target       `json:"origin"`
	Operation    archiveTargetOperation `json:"operation"`
}
type archiveTargetProjection struct {
	Version   int             `json:"version"`
	Scope     ControlScope    `json:"scope"`
	Revision  int64           `json:"revision"`
	Digest    string          `json:"digest"`
	Document  json.RawMessage `json:"document"`
	ActorID   uint64          `json:"actor_id"`
	UpdatedAt time.Time       `json:"updated_at"`
}
type archiveTargetControl struct {
	service  *Service
	archives *archive.Service
}

// BindArchiveControl 接管目标定义的唯一写入口；不自动导入、不启动传输或改写旧策略。
func (s *Service) BindArchiveControl(a *archive.Service) error {
	if s.opts.Control == nil || s.opts.Budget == nil {
		return controlError(503, "ARCHIVE_TARGET_CONTROL_UNAVAILABLE")
	}
	s.recoveryArchives = a
	if s.opts.RecoveryAnchor != nil {
		if e := a.BindRecoveryAnchor(s.opts.RecoveryAnchor); e != nil {
			return e
		}
	}
	c := &archiveTargetControl{s, a}
	if e := s.opts.Control.Bind(ControlArchiveTargets, ControlBinding{Validate: func(raw json.RawMessage) error { _, e := decodeArchiveTargets(raw); return e }, Apply: c.publish}); e != nil {
		return e
	}
	if e := a.BindTargetAuthority(c); e != nil {
		return e
	}
	return s.bindArchivePolicyControl(a)
}
func validTargetUUID(id string) bool {
	u, e := uuid.Parse(id)
	return e == nil && u != uuid.Nil && u.String() == id
}
func decodeArchiveTargets(raw []byte) (archiveTargetDocument, error) {
	var doc archiveTargetDocument
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&doc) != nil || d.Decode(new(any)) != io.EOF || doc.Version != 1 || len(doc.Entries) > 128 || len(doc.Origin) > 128 || len(doc.OriginDigest) != 64 {
		return doc, controlError(400, "ARCHIVE_TARGET_DOCUMENT_INVALID")
	}
	seen := map[string]bool{}
	for _, entry := range doc.Entries {
		if !validTargetUUID(entry.ID) || seen[entry.ID] || entry.Revision < 1 {
			return doc, controlError(400, "ARCHIVE_TARGET_DOCUMENT_INVALID")
		}
		seen[entry.ID] = true
		if e := archive.ValidateTargetSettings(entry.Settings); e != nil {
			return doc, e
		}
	}
	if (doc.Operation.Action != "adopt" || doc.Operation.Adoption == nil || doc.Operation.Change != nil) && (doc.Operation.Action != "save" || doc.Operation.Change == nil || doc.Operation.Adoption != nil) {
		return doc, controlError(400, "ARCHIVE_TARGET_DOCUMENT_INVALID")
	}
	return doc, nil
}
func (c *archiveTargetControl) legacy(ctx context.Context) ([]archive.Target, string, error) {
	targets, e := c.archives.LegacyTargets(ctx)
	if e != nil {
		return nil, "", e
	}
	raw, e := json.Marshal(struct {
		Targets []archive.Target `json:"targets"`
	}{targets})
	if e == nil {
		raw, e = canonicalControl(raw)
	}
	return targets, controlDigest(raw), e
}
func (c *archiveTargetControl) publish(ctx context.Context, p ControlPublication) error {
	if p.Scope != c.service.opts.Control.opts.Scope {
		return ErrStateUnavailable
	}
	doc, e := decodeArchiveTargets(p.Document)
	if e != nil {
		return e
	}
	if p.Revision == 1 {
		_, digest, e := c.legacy(ctx)
		if e != nil {
			return e
		}
		if digest != doc.OriginDigest {
			return controlError(409, "ARCHIVE_TARGET_LEGACY_CHANGED")
		}
	}
	_, e = updateArchiveControlProjection(ctx, c.service.opts.Budget, "archive-target-control", func(raw []byte) ([]byte, error) {
		// 首次投影可能已被更高期望版本取代，仍须核对接管来源，不能只检查 revision=1。
		if len(raw) == 0 {
			_, digest, err := c.legacy(ctx)
			if err != nil {
				return nil, err
			}
			if digest != doc.OriginDigest {
				return nil, controlError(409, "ARCHIVE_TARGET_LEGACY_CHANGED")
			}
		}
		if len(raw) > 0 {
			var old archiveTargetProjection
			if json.Unmarshal(raw, &old) != nil || old.Version != 1 || old.Scope != p.Scope || old.Revision > p.Revision || old.Revision == p.Revision && old.Digest != p.Digest {
				return nil, ErrStateUnavailable
			}
		}
		return json.Marshal(archiveTargetProjection{1, p.Scope, p.Revision, p.Digest, p.Document, p.ActorID, p.UpdatedAt})
	})
	return e
}
func targetEntry(doc archiveTargetDocument, id string) (archiveTargetEntry, bool) {
	for _, e := range doc.Entries {
		if e.ID == id {
			return e, true
		}
	}
	return archiveTargetEntry{}, false
}
func effectiveArchiveTarget(entry archiveTargetEntry, scope ControlScope) archive.Target {
	t := entry.Settings.Target(entry.ID, entry.Revision)
	raw, _ := json.Marshal(struct {
		Scope ControlScope
		Entry archiveTargetEntry
	}{scope, entry})
	t.ControlDigest = controlDigest(raw)
	return t
}
func (c *archiveTargetControl) load(ctx context.Context) (archive.TargetConfiguration, archiveTargetDocument, archiveTargetDocument, error) {
	out := archive.TargetConfiguration{Authority: "unavailable", State: "unavailable", Targets: []archive.Target{}}
	var desired, applied archiveTargetDocument
	fail := func(code string) (archive.TargetConfiguration, archiveTargetDocument, archiveTargetDocument, error) {
		out.Problem = code
		out.State = "unconfirmed"
		return out, desired, archiveTargetDocument{}, controlError(503, code)
	}
	ctrl := c.service.opts.Control
	if !ctrl.Ready(ctx) {
		return fail("STORAGE_CONTROL_SCHEMA_NOT_READY")
	}
	state, e := ctrl.read(ctx, ControlArchiveTargets)
	if e != nil {
		return fail("STORAGE_CONTROL_UNAVAILABLE")
	}
	out.Ready = c.service.opts.ControlWorkerEnabled && !ctrl.ConsumerExited()
	raw, e := readArchiveControlProjection(ctx, c.service.opts.Budget, "archive-target-control")
	if e != nil {
		return fail("ARCHIVE_TARGET_SNAPSHOT_UNAVAILABLE")
	}
	if state.State == "unconfigured" {
		if len(raw) > 0 {
			return fail("ARCHIVE_TARGET_RESTORE_CONFIRMATION_REQUIRED")
		}
		targets, digest, e := c.legacy(ctx)
		if e != nil {
			return fail("ARCHIVE_TARGET_LEGACY_UNAVAILABLE")
		}
		out.Authority, out.State, out.LegacyDigest, out.LegacyCount = "legacy", "legacy", digest, len(targets)
		for _, t := range targets {
			t.VerificationValid = false
			t.VerifiedAt = nil
			t.VerifiedFingerprint = ""
			out.Targets = append(out.Targets, t)
		}
		return out, desired, applied, nil
	}
	desired, e = decodeArchiveTargets(state.Document)
	if e != nil {
		return fail("ARCHIVE_TARGET_DOCUMENT_INVALID")
	}
	out.Authority, out.State, out.DesiredRevision, out.AppliedRevision, out.Problem = "database", state.State, state.DesiredRevision, state.AppliedRevision, state.Problem
	out.LegacyDigest, out.LegacyCount = desired.OriginDigest, len(desired.Origin)
	for _, entry := range desired.Entries {
		out.Targets = append(out.Targets, effectiveArchiveTarget(entry, state.Scope))
	}
	if ctrl.ConsumerExited() {
		out.Problem = "STORAGE_CONTROL_WORKER_UNAVAILABLE"
	}
	if len(raw) == 0 && state.AppliedRevision == 0 {
		return out, desired, applied, nil
	}
	var p archiveTargetProjection
	if json.Unmarshal(raw, &p) != nil || p.Version != 1 || p.Scope != state.Scope || p.Revision != state.AppliedRevision || p.Revision < 1 {
		return fail("ARCHIVE_TARGET_SNAPSHOT_UNCONFIRMED")
	}
	out.SnapshotRevision = &p.Revision
	version, e := ctrl.repo.Revision(ctx, state.Scope.ID(), string(ControlArchiveTargets), p.Revision)
	if e != nil || version.Digest != p.Digest || version.ActorID != p.ActorID || !version.CreatedAt.Equal(p.UpdatedAt) {
		return fail("ARCHIVE_TARGET_SNAPSHOT_UNCONFIRMED")
	}
	if _, e = verifiedControlDocument(p.Document, version.Digest); e != nil {
		return fail("ARCHIVE_TARGET_SNAPSHOT_UNCONFIRMED")
	}
	if _, e = verifiedControlDocument(version.Document, version.Digest); e != nil {
		return fail("ARCHIVE_TARGET_SNAPSHOT_UNCONFIRMED")
	}
	applied, e = decodeArchiveTargets(p.Document)
	if e != nil {
		return fail("ARCHIVE_TARGET_DOCUMENT_INVALID")
	}
	return out, desired, applied, nil
}

// readArchiveControlProjection 仅等待只读账本的瞬时占锁；不重试任何持久化或外部动作。
// 超时、损坏和身份不明仍返回失败，不能使用缓存绕过当前权威核验。
func readArchiveControlProjection(ctx context.Context, coordinator *budget.Coordinator, name string) ([]byte, error) {
	return updateArchiveControlProjection(ctx, coordinator, name, nil)
}

// 仅在尚未进入回调时等候锁。回调一旦开始，任何错误都交回正式 TaskBus 处理，不在此重放写入。
func updateArchiveControlProjection(ctx context.Context, coordinator *budget.Coordinator, name string, update func([]byte) ([]byte, error)) ([]byte, error) {
	bounded, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	for {
		entered := false
		raw, err := coordinator.Document(bounded, name, func(current []byte) ([]byte, error) {
			entered = true
			if update == nil {
				return nil, nil
			}
			return update(current)
		})
		if entered || !errors.Is(err, budget.ErrBusy) {
			return raw, err
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-bounded.Done():
			timer.Stop()
			return nil, bounded.Err()
		case <-timer.C:
		}
	}
}
func (c *archiveTargetControl) Configuration(ctx context.Context) (archive.TargetConfiguration, error) {
	out, _, _, e := c.load(ctx)
	return out, e
}
func (c *archiveTargetControl) Resolve(ctx context.Context, id string) (archive.Target, error) {
	out, desired, applied, e := c.load(ctx)
	if e != nil {
		return archive.Target{}, e
	}
	if out.Authority != "database" {
		return archive.Target{}, controlError(428, "ARCHIVE_TARGET_ADOPTION_REQUIRED")
	}
	want, ok := targetEntry(desired, id)
	if !ok {
		return archive.Target{}, controlError(404, "ARCHIVE_TARGET_NOT_FOUND")
	}
	active, ok := targetEntry(applied, id)
	if !ok || active != want {
		return archive.Target{}, controlError(409, "ARCHIVE_TARGET_PUBLICATION_PENDING")
	}
	return effectiveArchiveTarget(active, c.service.opts.Control.opts.Scope), nil
}
func (c *archiveTargetControl) requireWrite(ctx context.Context) error {
	if !auth.IsPlatformRootSession(ctx) {
		return ErrRootRequired
	}
	if !c.service.opts.ControlWorkerEnabled || !c.service.opts.Control.Ready(ctx) || c.service.opts.Control.ConsumerExited() {
		return controlError(503, "ARCHIVE_TARGET_CONTROL_UNAVAILABLE")
	}
	return nil
}
func (c *archiveTargetControl) replay(ctx context.Context, key string, op archiveTargetOperation) (archive.TargetMutation, bool, error) {
	row, e := c.service.opts.Control.repo.Request(ctx, c.service.opts.Control.opts.Scope.ID(), key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return archive.TargetMutation{}, false, nil
	}
	if e != nil {
		return archive.TargetMutation{}, true, controlError(503, "STORAGE_CONTROL_UNAVAILABLE")
	}
	if row.Domain != string(ControlArchiveTargets) || row.ActorID != reqctx.GetClaims(ctx).UserID {
		return archive.TargetMutation{}, true, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
	}
	if _, e = verifiedControlDocument(row.Document, row.Digest); e != nil {
		return archive.TargetMutation{}, true, e
	}
	doc, e := decodeArchiveTargets(row.Document)
	if e != nil {
		return archive.TargetMutation{}, true, e
	}
	a, _ := json.Marshal(doc.Operation)
	b, _ := json.Marshal(op)
	if !bytes.Equal(a, b) {
		return archive.TargetMutation{}, true, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
	}
	out, e := c.commit(ctx, row.Revision-1, key, doc)
	return out, true, e
}
func (c *archiveTargetControl) Adopt(ctx context.Context, in archive.TargetAdoption) (archive.TargetMutation, error) {
	if e := c.requireWrite(ctx); e != nil {
		return archive.TargetMutation{}, e
	}
	if !validTargetUUID(in.RequestID) {
		return archive.TargetMutation{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	op := archiveTargetOperation{Action: "adopt", Adoption: &in}
	if out, found, e := c.replay(ctx, in.RequestID, op); found {
		return out, e
	}
	origin, digest, e := c.legacy(ctx)
	if e != nil {
		return archive.TargetMutation{}, e
	}
	if in.LegacyDigest != digest {
		return archive.TargetMutation{}, controlError(409, "ARCHIVE_TARGET_LEGACY_CHANGED")
	}
	doc := archiveTargetDocument{Version: 1, Entries: []archiveTargetEntry{}, OriginDigest: digest, Origin: origin, Operation: op}
	for _, t := range origin {
		doc.Entries = append(doc.Entries, archiveTargetEntry{t.ID, t.Revision + 1, archive.SettingsOfTarget(t)})
	}
	return c.commit(ctx, 0, in.RequestID, doc)
}
func (c *archiveTargetControl) Save(ctx context.Context, in archive.TargetChange) (archive.TargetMutation, error) {
	if e := c.requireWrite(ctx); e != nil {
		return archive.TargetMutation{}, e
	}
	if !validTargetUUID(in.RequestID) || in.ExpectedRevision < 1 || in.ExpectedTargetRevision < 0 || in.TargetID != "" && !validTargetUUID(in.TargetID) {
		return archive.TargetMutation{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	if e := archive.ValidateTargetSettings(in.Settings); e != nil {
		return archive.TargetMutation{}, e
	}
	op := archiveTargetOperation{Action: "save", Change: &in}
	if out, found, e := c.replay(ctx, in.RequestID, op); found {
		return out, e
	}
	state, e := c.service.opts.Control.read(ctx, ControlArchiveTargets)
	if e != nil {
		return archive.TargetMutation{}, e
	}
	if state.State == "unconfigured" {
		return archive.TargetMutation{}, controlError(428, "ARCHIVE_TARGET_ADOPTION_REQUIRED")
	}
	if state.DesiredRevision != in.ExpectedRevision {
		return archive.TargetMutation{}, controlError(409, "STORAGE_CONTROL_REVISION_CONFLICT")
	}
	doc, e := decodeArchiveTargets(state.Document)
	if e != nil {
		return archive.TargetMutation{}, e
	}
	id := in.TargetID
	if id == "" {
		id = in.RequestID
	}
	old, exists := targetEntry(doc, id)
	if exists && (in.TargetID == "" || old.Revision != in.ExpectedTargetRevision) || !exists && (in.TargetID != "" || in.ExpectedTargetRevision != 0) {
		return archive.TargetMutation{}, controlError(409, "ARCHIVE_REVISION_CONFLICT")
	}
	entry := archiveTargetEntry{id, in.ExpectedTargetRevision + 1, in.Settings}
	if exists {
		for i := range doc.Entries {
			if doc.Entries[i].ID == id {
				doc.Entries[i] = entry
			}
		}
	} else {
		doc.Entries = append(doc.Entries, entry)
	}
	sort.Slice(doc.Entries, func(i, j int) bool { return doc.Entries[i].ID < doc.Entries[j].ID })
	doc.Operation = op
	return c.commit(ctx, in.ExpectedRevision, in.RequestID, doc)
}
func (c *archiveTargetControl) commit(ctx context.Context, revision int64, key string, doc archiveTargetDocument) (archive.TargetMutation, error) {
	raw, e := json.Marshal(doc)
	if e != nil {
		return archive.TargetMutation{}, e
	}
	state, e := c.service.opts.Control.Save(ctx, ControlInput{key, revision, ControlArchiveTargets, raw})
	if e != nil {
		return archive.TargetMutation{}, e
	}
	out, _, _, _ := c.load(ctx)
	id := ""
	if doc.Operation.Change != nil {
		id = doc.Operation.Change.TargetID
		if id == "" {
			id = key
		}
	}
	return archive.TargetMutation{RequestID: key, OperationRevision: state.OperationRevision, TargetID: id, Configuration: out}, nil
}
func (c *archiveTargetControl) Request(ctx context.Context, key string) (archive.TargetMutation, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return archive.TargetMutation{}, ErrRootRequired
	}
	if !validTargetUUID(key) {
		return archive.TargetMutation{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	row, e := c.service.opts.Control.repo.Request(ctx, c.service.opts.Control.opts.Scope.ID(), key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return archive.TargetMutation{}, controlError(404, "STORAGE_CONTROL_REQUEST_NOT_FOUND")
	}
	if e != nil {
		return archive.TargetMutation{}, controlError(503, "STORAGE_CONTROL_UNAVAILABLE")
	}
	if row.Domain != string(ControlArchiveTargets) || row.ActorID != reqctx.GetClaims(ctx).UserID {
		return archive.TargetMutation{}, controlError(403, "STORAGE_CONTROL_REQUEST_FORBIDDEN")
	}
	if _, e = verifiedControlDocument(row.Document, row.Digest); e != nil {
		return archive.TargetMutation{}, e
	}
	doc, e := decodeArchiveTargets(row.Document)
	if e != nil {
		return archive.TargetMutation{}, e
	}
	out, _, _, _ := c.load(ctx)
	id := ""
	if doc.Operation.Change != nil {
		id = doc.Operation.Change.TargetID
		if id == "" {
			id = key
		}
	}
	return archive.TargetMutation{RequestID: key, OperationRevision: row.Revision, TargetID: id, Configuration: out}, nil
}
func (c *archiveTargetControl) Confirm(ctx context.Context, t archive.Target, save func(archive.Target) error) error {
	current, e := c.Resolve(ctx, t.ID)
	if e != nil {
		return e
	}
	if current.ControlDigest != t.ControlDigest {
		return controlError(409, "ARCHIVE_REVISION_CONFLICT")
	}
	return c.service.opts.Control.repo.WithCurrent(ctx, c.service.opts.Control.opts.Scope.ID(), string(ControlArchiveTargets), func(row *m.ControlConfiguration) error {
		if row == nil || row.AppliedRevision < 1 {
			return ErrStateUnavailable
		}
		if _, e := verifiedControlDocument(row.Document, row.Digest); e != nil {
			return e
		}
		doc, e := decodeArchiveTargets(row.Document)
		if e != nil {
			return e
		}
		entry, ok := targetEntry(doc, t.ID)
		if !ok || effectiveArchiveTarget(entry, c.service.opts.Control.opts.Scope).ControlDigest != t.ControlDigest {
			return controlError(409, "ARCHIVE_REVISION_CONFLICT")
		}
		return save(current)
	})
}

func (c *archiveTargetControl) ChangeDirectories() []string {
	return []string{c.service.opts.Budget.ChangeDirectory()}
}
