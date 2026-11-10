package storagegov

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	storagerecovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	backup "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type backupPolicyOperation struct {
	Action   string                 `json:"action"`
	Change   *backup.PolicyChange   `json:"change,omitempty"`
	Adoption *backup.PolicyAdoption `json:"adoption,omitempty"`
}
type backupPolicyDocument struct {
	Recovery       *RecoveryStamp        `json:"recovery,omitempty"`
	Version        int                   `json:"version"`
	PreviousDigest string                `json:"previous_digest"`
	Origin         []backup.PolicyEntry  `json:"origin"`
	OriginDigest   string                `json:"origin_digest"`
	Policies       []backup.PolicyEntry  `json:"policies"`
	Operation      backupPolicyOperation `json:"operation"`
}
type backupPolicyProjection struct {
	Version   int             `json:"version"`
	Scope     ControlScope    `json:"scope"`
	Revision  int64           `json:"revision"`
	Digest    string          `json:"digest"`
	Document  json.RawMessage `json:"document"`
	ActorID   uint64          `json:"actor_id"`
	UpdatedAt time.Time       `json:"updated_at"`
}
type backupPolicyControl struct {
	service     *Service
	repo        *repo.BackupPolicyRepository
	environment string
	fingerprint func() string
}

func entriesOfBackup(rows []model.BackupPolicy) []backup.PolicyEntry {
	out := []backup.PolicyEntry{}
	for _, p := range rows {
		out = append(out, backup.PolicyEntry{UUID: p.UUID.String(), ExecutionFingerprint: p.ExecutionFingerprint, Revision: p.ControlRevision, Settings: backup.SettingsOf(p), Enabled: p.Enabled, Current: p.IsCurrent})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UUID < out[j].UUID })
	return out
}
func backupEntriesDigest(rows []backup.PolicyEntry) string {
	raw, _ := json.Marshal(rows)
	return controlDigest(raw)
}
func decodeBackupPolicies(raw []byte, env string) (backupPolicyDocument, error) {
	var d backupPolicyDocument
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&d) != nil || dec.Decode(new(any)) != io.EOF || d.Version != 1 || len(d.Policies) > 200 || len(d.Origin) > 200 || len(d.OriginDigest) != 64 || len(d.PreviousDigest) != 64 || backupEntriesDigest(d.Origin) != d.OriginDigest {
		return d, controlError(400, "BACKUP_POLICY_DOCUMENT_INVALID")
	}
	ids, names := map[string]bool{}, map[string]bool{}
	current := 0
	for _, p := range d.Policies {
		if !validTargetUUID(p.UUID) || len(p.ExecutionFingerprint) != 64 || p.Revision < 1 || ids[p.UUID] || names[strings.ToLower(p.Settings.Name)] || backup.ValidateSettings(p.Settings, env) != nil || p.Current && !p.Enabled {
			return d, controlError(400, "BACKUP_POLICY_DOCUMENT_INVALID")
		}
		ids[p.UUID] = true
		names[strings.ToLower(p.Settings.Name)] = true
		if p.Current {
			current++
		}
	}
	if current > 1 || (d.Operation.Action != "adopt" || d.Operation.Adoption == nil || d.Operation.Change != nil) && (d.Operation.Action != "change" || d.Operation.Change == nil || d.Operation.Adoption != nil) {
		return d, controlError(400, "BACKUP_POLICY_DOCUMENT_INVALID")
	}
	return d, nil
}
func (s *Service) BindDatabaseBackupControl(p *backup.PolicyService, j *backup.JobService, env string) error {
	if s.opts.Control == nil || s.opts.Budget == nil || s.opts.DB == nil || p == nil || j == nil {
		return ErrStateUnavailable
	}
	c := &backupPolicyControl{s, repo.NewBackupPolicyRepository(s.opts.DB), env, j.ExecutionFingerprint}
	s.recoveryBackups = c
	if e := s.opts.Control.Bind(ControlBackupPolicies, ControlBinding{Validate: func(raw json.RawMessage) error { _, e := decodeBackupPolicies(raw, env); return e }, Commit: c.commitRows, Apply: c.publish}); e != nil {
		return e
	}
	if e := p.BindAuthority(c); e != nil {
		return e
	}
	if e := j.BindAuthority(c); e != nil {
		return e
	}
	j.SetCleanupChangeNotifier(func() { s.PublishChange("backup_cleanup_changed") })
	return nil
}
func (c *backupPolicyControl) rows(ctx context.Context, db *gorm.DB) ([]model.BackupPolicy, error) {
	if !db.Migrator().HasColumn(&model.BackupPolicy{}, "control_revision") || !db.Migrator().HasColumn(&model.BackupPolicy{}, "control_scope_id") {
		return nil, controlError(503, "BACKUP_POLICY_SCHEMA_NOT_READY")
	}
	rows, e := c.repo.ControlRows(ctx, db)
	if e != nil {
		return nil, e
	}
	if len(rows) > 200 {
		return nil, controlError(503, "BACKUP_POLICY_CATALOG_LIMIT")
	}
	scope := c.service.opts.Control.opts.Scope.ID()
	for _, p := range rows {
		if p.UUID == uuid.Nil || p.ControlScopeID != "" && p.ControlScopeID != scope {
			return nil, controlError(409, "BACKUP_POLICY_OWNER_MISMATCH")
		}
	}
	return rows, nil
}
func (c *backupPolicyControl) commitRows(ctx context.Context, tx *gorm.DB, raw json.RawMessage) error {
	d, e := decodeBackupPolicies(raw, c.environment)
	if e != nil {
		return e
	}
	if e = c.repo.LockControlCatalog(ctx, tx); e != nil {
		return e
	}
	rows, e := c.rows(ctx, tx)
	if e != nil {
		return e
	}
	if backupEntriesDigest(entriesOfBackup(rows)) != d.PreviousDigest {
		return controlError(409, "BACKUP_POLICY_ROWS_CHANGED")
	}
	previous := map[string]model.BackupPolicy{}
	for _, row := range rows {
		previous[row.UUID.String()] = row
	}
	next := []model.BackupPolicy{}
	for _, entry := range d.Policies {
		row := previous[entry.UUID]
		row.UUID = uuid.MustParse(entry.UUID)
		backup.ApplySettings(&row, entry.Settings)
		row.ControlRevision = entry.Revision
		row.ExecutionFingerprint = entry.ExecutionFingerprint
		row.Enabled = entry.Enabled
		row.IsCurrent = entry.Current
		next = append(next, row)
	}
	if len(next) < len(rows) {
		return controlError(400, "BACKUP_POLICY_DELETE_UNSUPPORTED")
	}
	return c.repo.WriteControlRows(ctx, tx, c.service.opts.Control.opts.Scope.ID(), strconv.FormatUint(reqctx.GetUserID(ctx), 10), next)
}
func (c *backupPolicyControl) publish(ctx context.Context, p ControlPublication) error {
	if p.Scope != c.service.opts.Control.opts.Scope {
		return ErrStateUnavailable
	}
	d, e := decodeBackupPolicies(p.Document, c.environment)
	if e != nil {
		return e
	}
	// 发布回调处在同一正式 SQL 事务内，只做受控投影；行快照已由 Commit 原子耦合。
	_, e = updateArchiveControlProjection(ctx, c.service.opts.Budget, "backup-policy-control", func(raw []byte) ([]byte, error) {
		if len(raw) > 0 {
			var old backupPolicyProjection
			if json.Unmarshal(raw, &old) != nil || old.Version != 1 || old.Scope != p.Scope || old.Revision > p.Revision || old.Revision == p.Revision && old.Digest != p.Digest {
				return nil, ErrStateUnavailable
			}
		}
		_ = d
		return json.Marshal(backupPolicyProjection{1, p.Scope, p.Revision, p.Digest, p.Document, p.ActorID, p.UpdatedAt})
	})
	return e
}
func (c *backupPolicyControl) load(ctx context.Context) (backup.PolicyConfiguration, ControlState, backupPolicyDocument, backupPolicyDocument, error) {
	out := backup.PolicyConfiguration{Authority: "unavailable", State: "unavailable", Policies: []backup.PolicyEntry{}, ExecutionReady: map[string]bool{}}
	var state ControlState
	var desired, applied backupPolicyDocument
	fail := func(code string) (backup.PolicyConfiguration, ControlState, backupPolicyDocument, backupPolicyDocument, error) {
		out.Authority = "unavailable"
		out.State = "unconfirmed"
		out.Problem = code
		out.Ready = false
		return out, state, desired, applied, controlError(503, code)
	}
	ctrl := c.service.opts.Control
	if !ctrl.Ready(ctx) {
		return fail("STORAGE_CONTROL_SCHEMA_NOT_READY")
	}
	// 同一次有界 SQL 读锁获取配置、业务行及投影，避免并发保存产生混合版本误报。
	var rows []model.BackupPolicy
	var raw []byte
	e := ctrl.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return ctrl.repo.WithCurrentTx(ctx, tx, ctrl.opts.Scope.ID(), string(ControlBackupPolicies), func(row *cm.ControlConfiguration) error {
			state = ControlState{Scope: ctrl.opts.Scope, Domain: ControlBackupPolicies, State: "unconfigured"}
			if row != nil {
				if row.HostID != ctrl.opts.Scope.HostID || row.FilesystemID != ctrl.opts.Scope.FilesystemID || row.InstanceID != ctrl.opts.Scope.InstanceID {
					return ErrStateUnavailable
				}
				if _, err := verifiedControlDocument(row.Document, row.Digest); err != nil {
					return err
				}
				state = ctrl.state(row)
			}
			if err := c.repo.LockControlCatalog(ctx, tx); err != nil {
				return err
			}
			var err error
			rows, err = c.rows(ctx, tx)
			if err != nil {
				return err
			}
			raw, err = readArchiveControlProjection(ctx, c.service.opts.Budget, "backup-policy-control")
			return err
		})
	})
	if e != nil {
		return fail("BACKUP_POLICY_ROWS_UNAVAILABLE")
	}
	out.Ready = c.service.opts.ControlWorkerEnabled && !ctrl.ConsumerExited()
	if state.State == "unconfigured" {
		if len(raw) > 0 {
			return fail("BACKUP_POLICY_RESTORE_CONFIRMATION_REQUIRED")
		}
		for _, p := range rows {
			if p.ControlScopeID != "" || p.ControlRevision != 0 {
				return fail("BACKUP_POLICY_RESTORE_CONFIRMATION_REQUIRED")
			}
		}
		out.Authority = "legacy"
		out.State = "legacy"
		out.Policies = entriesOfBackup(rows)
		out.LegacyDigest = backupEntriesDigest(out.Policies)
		return out, state, desired, applied, nil
	}
	desired, e = decodeBackupPolicies(state.Document, c.environment)
	if e != nil {
		return fail("BACKUP_POLICY_DOCUMENT_INVALID")
	}
	out.Authority = "database"
	out.State = state.State
	out.DesiredRevision = state.DesiredRevision
	out.AppliedRevision = state.AppliedRevision
	out.LegacyDigest = desired.OriginDigest
	out.Policies = desired.Policies
	out.Problem = state.Problem
	if backupEntriesDigest(entriesOfBackup(rows)) != backupEntriesDigest(desired.Policies) {
		return fail("BACKUP_POLICY_ROWS_UNCONFIRMED")
	}
	if len(raw) == 0 && state.AppliedRevision == 0 {
		return out, state, desired, applied, nil
	}
	var p backupPolicyProjection
	if json.Unmarshal(raw, &p) != nil || p.Version != 1 || p.Scope != state.Scope || p.Revision != state.AppliedRevision || p.Revision < 1 {
		return fail("BACKUP_POLICY_SNAPSHOT_UNCONFIRMED")
	}
	out.SnapshotRevision = &p.Revision
	version, e := ctrl.repo.Revision(ctx, state.Scope.ID(), string(ControlBackupPolicies), p.Revision)
	if e != nil || version.Digest != p.Digest || version.ActorID != p.ActorID || !version.CreatedAt.Equal(p.UpdatedAt) {
		return fail("BACKUP_POLICY_SNAPSHOT_UNCONFIRMED")
	}
	if _, e = verifiedControlDocument(p.Document, p.Digest); e != nil {
		return fail("BACKUP_POLICY_SNAPSHOT_UNCONFIRMED")
	}
	if _, e = verifiedControlDocument(version.Document, version.Digest); e != nil {
		return fail("BACKUP_POLICY_SNAPSHOT_UNCONFIRMED")
	}
	applied, e = decodeBackupPolicies(p.Document, c.environment)
	if e != nil {
		return fail("BACKUP_POLICY_DOCUMENT_INVALID")
	}
	for _, entry := range desired.Policies {
		out.ExecutionReady[entry.UUID] = false
		for _, active := range applied.Policies {
			if active == entry && entry.Enabled && entry.ExecutionFingerprint == c.fingerprint() {
				out.ExecutionReady[entry.UUID] = true
			}
		}
	}
	return out, state, desired, applied, nil
}
func (c *backupPolicyControl) Configuration(ctx context.Context) (backup.PolicyConfiguration, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return backup.PolicyConfiguration{}, ErrRootRequired
	}
	out, _, _, _, e := c.load(ctx)
	return out, e
}
func (c *backupPolicyControl) writable(ctx context.Context) (backup.PolicyConfiguration, backupPolicyDocument, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return backup.PolicyConfiguration{}, backupPolicyDocument{}, ErrRootRequired
	}
	out, _, d, _, e := c.load(ctx)
	if e != nil {
		return out, d, e
	}
	if !out.Ready {
		return out, d, controlError(503, "STORAGE_CONTROL_WORKER_UNAVAILABLE")
	}
	return out, d, nil
}
func (c *backupPolicyControl) Adopt(ctx context.Context, in backup.PolicyAdoption) (backup.PolicyMutation, error) {
	if !in.Confirm || !validTargetUUID(in.RequestID) {
		return backup.PolicyMutation{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	op := backupPolicyOperation{Action: "adopt", Adoption: &in}
	if out, ok, e := c.replay(ctx, in.RequestID, op); ok {
		return out, e
	}
	config, _, e := c.writable(ctx)
	if e != nil {
		return backup.PolicyMutation{}, e
	}
	if config.Authority != "legacy" || config.LegacyDigest != in.LegacyDigest {
		return backup.PolicyMutation{}, controlError(409, "BACKUP_POLICY_LEGACY_CHANGED")
	}
	origin := config.Policies
	entries := append([]backup.PolicyEntry{}, origin...)
	for i := range entries {
		entries[i].Revision = 1
		entries[i].ExecutionFingerprint = c.fingerprint()
		entries[i].Settings.DrillEnabled = false
		entries[i].Enabled = false
		entries[i].Current = false
	}
	d := backupPolicyDocument{nil, 1, in.LegacyDigest, origin, in.LegacyDigest, entries, op}
	return c.save(ctx, 0, in.RequestID, d)
}
func (c *backupPolicyControl) Change(ctx context.Context, in backup.PolicyChange) (backup.PolicyMutation, error) {
	if !validTargetUUID(in.RequestID) || in.ExpectedRevision < 1 || !in.Confirm {
		return backup.PolicyMutation{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	if in.Action != "save" && in.Action != "status" && in.Action != "current" {
		return backup.PolicyMutation{}, controlError(400, "BACKUP_POLICY_ACTION_INVALID")
	}
	if in.Action == "save" {
		if in.Settings == nil || in.Enabled != nil || backup.ValidateSettings(*in.Settings, c.environment) != nil {
			return backup.PolicyMutation{}, controlError(400, "BACKUP_POLICY_SETTINGS_INVALID")
		}
	} else if in.Settings != nil || in.Action == "status" && in.Enabled == nil || in.Action == "current" && in.Enabled != nil {
		return backup.PolicyMutation{}, controlError(400, "BACKUP_POLICY_ACTION_INVALID")
	}
	op := backupPolicyOperation{Action: "change", Change: &in}
	if out, ok, e := c.replay(ctx, in.RequestID, op); ok {
		return out, e
	}
	config, d, e := c.writable(ctx)
	if e != nil {
		return backup.PolicyMutation{}, e
	}
	if config.Authority != "database" {
		return backup.PolicyMutation{}, controlError(428, "BACKUP_POLICY_ADOPTION_REQUIRED")
	}
	if config.DesiredRevision != in.ExpectedRevision {
		return backup.PolicyMutation{}, controlError(409, "STORAGE_CONTROL_REVISION_CONFLICT")
	}
	d.PreviousDigest = backupEntriesDigest(d.Policies)
	d.Operation = op
	id := in.PolicyUUID
	if id == "" {
		id = in.RequestID
	}
	index := -1
	for i, p := range d.Policies {
		if p.UUID == id {
			index = i
			break
		}
	}
	if index < 0 {
		if in.Action != "save" || in.PolicyUUID != "" || in.ExpectedPolicyRevision != 0 {
			return backup.PolicyMutation{}, controlError(404, "BACKUP_POLICY_NOT_FOUND")
		}
		d.Policies = append(d.Policies, backup.PolicyEntry{UUID: id})
		index = len(d.Policies) - 1
	}
	p := &d.Policies[index]
	if p.Revision != in.ExpectedPolicyRevision {
		return backup.PolicyMutation{}, controlError(409, "BACKUP_POLICY_REVISION_CONFLICT")
	}
	p.Revision++
	switch in.Action {
	case "save":
		p.Settings = *in.Settings
		p.ExecutionFingerprint = c.fingerprint()
		p.Enabled = false
		p.Current = false
	case "status":
		if *in.Enabled && p.ExecutionFingerprint != c.fingerprint() {
			return backup.PolicyMutation{}, controlError(409, "BACKUP_POLICY_SOURCE_CHANGED")
		}
		p.Enabled = *in.Enabled
		if !p.Enabled {
			p.Current = false
		}
	case "current":
		if !p.Enabled {
			return backup.PolicyMutation{}, controlError(409, "BACKUP_POLICY_DISABLED")
		}
		for i := range d.Policies {
			if i != index && d.Policies[i].Current {
				d.Policies[i].Current = false
				d.Policies[i].Revision++
			}
		}
		p.Current = true
	}
	sort.Slice(d.Policies, func(i, j int) bool { return d.Policies[i].UUID < d.Policies[j].UUID })
	return c.save(ctx, in.ExpectedRevision, in.RequestID, d)
}
func (c *backupPolicyControl) save(ctx context.Context, revision int64, key string, d backupPolicyDocument) (backup.PolicyMutation, error) {
	raw, e := json.Marshal(d)
	if e != nil {
		return backup.PolicyMutation{}, e
	}
	_, e = c.service.opts.Control.Save(ctx, ControlInput{key, revision, ControlBackupPolicies, raw})
	if e != nil {
		return backup.PolicyMutation{}, e
	}
	return c.Request(ctx, key)
}
func (c *backupPolicyControl) replay(ctx context.Context, key string, op backupPolicyOperation) (backup.PolicyMutation, bool, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return backup.PolicyMutation{}, true, ErrRootRequired
	}
	row, e := c.service.opts.Control.repo.Request(ctx, c.service.opts.Control.opts.Scope.ID(), key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return backup.PolicyMutation{}, false, nil
	}
	if e != nil {
		return backup.PolicyMutation{}, true, ErrStateUnavailable
	}
	if row.Domain != string(ControlBackupPolicies) || row.ActorID != reqctx.GetUserID(ctx) {
		return backup.PolicyMutation{}, true, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
	}
	d, e := decodeBackupPolicies(row.Document, c.environment)
	if e != nil {
		return backup.PolicyMutation{}, true, e
	}
	a, _ := json.Marshal(d.Operation)
	b, _ := json.Marshal(op)
	if !bytes.Equal(a, b) {
		return backup.PolicyMutation{}, true, controlError(409, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT")
	}
	out, e := c.Request(ctx, key)
	return out, true, e
}
func (c *backupPolicyControl) Request(ctx context.Context, key string) (backup.PolicyMutation, error) {
	if !auth.IsPlatformRootSession(ctx) {
		return backup.PolicyMutation{}, ErrRootRequired
	}
	if !validTargetUUID(key) {
		return backup.PolicyMutation{}, controlError(400, "STORAGE_CONTROL_PRECONDITION_REQUIRED")
	}
	row, e := c.service.opts.Control.repo.Request(ctx, c.service.opts.Control.opts.Scope.ID(), key)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return backup.PolicyMutation{}, controlError(404, "STORAGE_CONTROL_REQUEST_NOT_FOUND")
	}
	if e != nil {
		return backup.PolicyMutation{}, ErrStateUnavailable
	}
	if row.Domain != string(ControlBackupPolicies) || row.ActorID != reqctx.GetUserID(ctx) {
		return backup.PolicyMutation{}, controlError(403, "STORAGE_CONTROL_REQUEST_FORBIDDEN")
	}
	if _, e = verifiedControlDocument(row.Document, row.Digest); e != nil {
		return backup.PolicyMutation{}, e
	}
	d, e := decodeBackupPolicies(row.Document, c.environment)
	if e != nil {
		return backup.PolicyMutation{}, e
	}
	out, _, _, _, _ := c.load(ctx)
	id := ""
	if d.Operation.Change != nil {
		id = d.Operation.Change.PolicyUUID
		if id == "" {
			id = key
		}
	}
	return backup.PolicyMutation{RequestID: key, OperationRevision: row.Revision, PolicyUUID: id, Configuration: out}, nil
}
func (c *backupPolicyControl) Guard(ctx context.Context, id uint64, revision int64, operation string, action func(context.Context, *gorm.DB, *model.BackupPolicy) error) error {
	if operation != "start" && operation != "scheduled" && operation != "cleanup" && operation != "drill" {
		return controlError(400, "BACKUP_POLICY_ACTION_INVALID")
	}
	run, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	config, state, desired, applied, e := c.load(run)
	if e != nil {
		return e
	}
	if config.Authority != "database" {
		return controlError(428, "BACKUP_POLICY_ADOPTION_REQUIRED")
	}
	return c.service.opts.Control.repo.DB.WithContext(run).Transaction(func(tx *gorm.DB) error {
		return c.service.opts.Control.WithExecutionTx(run, tx, ControlBackupPolicies, func(row *cm.ControlConfiguration) error {
			policy, e := c.guardedPolicy(run, tx, id, revision, state, desired, applied, row)
			if e != nil {
				return e
			}
			if !policy.Enabled || operation == "scheduled" && !policy.IsCurrent || operation == "drill" && !policy.DrillEnabled {
				return controlError(409, "BACKUP_POLICY_DISABLED")
			}
			return action(run, tx, policy)
		})
	})
}

// guardedPolicy is shared by execution and protection, using the same configuration/catalog lock order.
func (c *backupPolicyControl) guardedPolicy(run context.Context, tx *gorm.DB, id uint64, revision int64, state ControlState, desired, applied backupPolicyDocument, row *cm.ControlConfiguration) (*model.BackupPolicy, error) {
	if row == nil || row.Revision != state.DesiredRevision || row.AppliedRevision != state.AppliedRevision || row.Digest != state.Digest || row.HostID != state.Scope.HostID || row.FilesystemID != state.Scope.FilesystemID || row.InstanceID != state.Scope.InstanceID {
		return nil, controlError(409, "BACKUP_POLICY_CHANGED")
	}
	if _, e := verifiedControlDocument(row.Document, row.Digest); e != nil {
		return nil, e
	}
	if e := c.repo.LockControlCatalog(run, tx); e != nil {
		return nil, e
	}
	rows, e := c.rows(run, tx)
	if e != nil {
		return nil, e
	}
	if backupEntriesDigest(entriesOfBackup(rows)) != backupEntriesDigest(desired.Policies) {
		return nil, controlError(409, "BACKUP_POLICY_ROWS_CHANGED")
	}
	var policy *model.BackupPolicy
	for i := range rows {
		if rows[i].ID == id {
			policy = &rows[i]
			break
		}
	}
	if policy == nil {
		return nil, controlError(404, "BACKUP_POLICY_NOT_FOUND")
	}
	var approved *backup.PolicyEntry
	for i := range applied.Policies {
		if applied.Policies[i].UUID == policy.UUID.String() {
			approved = &applied.Policies[i]
			break
		}
	}
	expected := entriesOfBackup([]model.BackupPolicy{*policy})[0]
	if approved == nil || *approved != expected || revision > 0 && policy.ControlRevision != revision {
		return nil, controlError(409, "BACKUP_POLICY_PUBLICATION_PENDING")
	}
	if policy.ExecutionFingerprint != c.fingerprint() {
		return nil, controlError(409, "BACKUP_POLICY_SOURCE_CHANGED")
	}
	return policy, nil
}

// RetainGuard only permits adding protection while automatic execution and this policy are paused.
// It bypasses the open-only execution fence for this fixed safety operation, without enabling any executor.
func (c *backupPolicyControl) RetainGuard(ctx context.Context, id uint64, expectedFence string, action func(context.Context, *gorm.DB, *model.BackupPolicy, string) error) error {
	run, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	config, state, desired, applied, e := c.load(run)
	if e != nil {
		return e
	}
	if config.Authority != "database" {
		return controlError(428, "BACKUP_POLICY_ADOPTION_REQUIRED")
	}
	return c.service.withRecoveryGuard(run, func() error {
		return c.service.opts.DB.WithContext(run).Transaction(func(tx *gorm.DB) error {
			ctrl := c.service.opts.Control
			return ctrl.repo.WithExecutionTx(run, tx, ctrl.opts.Scope.ID(), func(execution *cm.ControlConfiguration) error {
				status, _, e := c.service.executionStateTx(run, tx, execution)
				if e != nil {
					return e
				}
				if execution == nil || status.Mode != "paused" || !status.Confirmed || status.Recovery.Epoch != "" && status.Recovery.State != "acknowledged" {
					return controlError(423, "BACKUP_RETAIN_PAUSE_REQUIRED")
				}
				return ctrl.repo.WithCurrentTx(run, tx, ctrl.opts.Scope.ID(), string(ControlBackupPolicies), func(row *cm.ControlConfiguration) error {
					policy, e := c.guardedPolicy(run, tx, id, 0, state, desired, applied, row)
					if e != nil {
						return e
					}
					if policy.Enabled || policy.IsCurrent || policy.DrillEnabled {
						return controlError(423, "BACKUP_RETAIN_POLICY_PAUSE_REQUIRED")
					}
					binding, e := c.RetentionReviewBinding(run)
					if e != nil {
						return e
					}
					fence := controlDigest([]byte(ctrl.opts.Scope.ID() + "/" + execution.Digest + "/" + row.Digest + "/" + policy.ExecutionFingerprint + "/" + binding))
					if expectedFence != "" && expectedFence != fence {
						return controlError(409, "BACKUP_RETAIN_FENCE_CHANGED")
					}
					return action(run, tx, policy, fence)
				})
			})
		})
	})
}

func (c *backupPolicyControl) RetentionLedger() *storagerecovery.Store {
	return c.service.opts.RecoveryAnchor
}

func (c *backupPolicyControl) RetentionScope() string { return c.service.opts.Control.opts.Scope.ID() }

// RetentionReviewBinding survives SQL/runtime rollback and changes before a
// managed restore overwrites data. No database or credential value is exposed.
func (c *backupPolicyControl) RetentionReviewBinding(ctx context.Context) (string, error) {
	scope := c.RetentionScope()
	if c.service.opts.RecoveryAnchor == nil {
		return "", controlError(503, "BACKUP_RETAIN_LEDGER_UNAVAILABLE")
	}
	if e := c.service.opts.RecoveryAnchor.RetentionReady(ctx); e != nil {
		return "", controlError(503, "BACKUP_RETAIN_LEDGER_UNAVAILABLE")
	}
	var out string
	e := c.service.opts.RecoveryAnchor.WithEpoch(ctx, func(epoch storagerecovery.Epoch) error {
		out = controlDigest([]byte(scope + "/" + epoch.Protocol + "/" + epoch.Installation + "/" + epoch.AnchorID + "/" + epoch.Epoch))
		return nil
	})
	if e != nil {
		return "", controlError(503, "STORAGE_RECOVERY_ANCHOR_UNAVAILABLE")
	}
	return out, nil
}

// RetentionImportGuard runs only under the caller's exclusive recovery-root lock.
// The pinned epoch prevents a nested flock while retaining publication checks.
func (c *backupPolicyControl) RetentionImportGuard(ctx context.Context, epoch storagerecovery.Epoch, fn func(context.Context, *gorm.DB) error) error {
	if !c.service.opts.Control.Ready(ctx) {
		return controlError(503, "STORAGE_CONTROL_SCHEMA_NOT_READY")
	}
	return c.service.opts.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return c.service.opts.Control.repo.WithExecutionTx(ctx, tx, c.RetentionScope(), func(row *cm.ControlConfiguration) error {
			state, _, e := c.service.executionStateTxPinned(ctx, tx, row, &epoch)
			if e != nil {
				return e
			}
			if row == nil || state.Mode != "paused" || !state.Confirmed || state.Recovery.Epoch != "" && state.Recovery.State != "acknowledged" {
				return controlError(423, "BACKUP_HISTORY_PAUSE_REQUIRED")
			}
			return fn(ctx, tx)
		})
	})
}

func (c *backupPolicyControl) RetentionImportPaused(ctx context.Context) bool {
	v, e := c.service.ExecutionConfiguration(ctx)
	return e == nil && v.Mode == "paused" && v.Confirmed && (v.Recovery.Epoch == "" || v.Recovery.State == "acknowledged")
}
