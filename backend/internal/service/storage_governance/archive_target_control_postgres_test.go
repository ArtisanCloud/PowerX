package storagegov

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func targetNative(t *testing.T) (*archiveTargetControl, *ControlService, archive.Target, string) {
	t.Helper()
	s, c, _, _ := monitoringNative(t)
	dir := t.TempDir()
	a := archive.NewService(context.Background(), archive.Config{Directory: dir, Instance: "owned", ExecutionNodeID: "owned", S3Credentials: map[string]archive.S3Credential{"owned": {AccessKey: "fixture", SecretKey: "fixture-secret"}}}, archive.Options{})
	t.Cleanup(a.Close)
	target, e := a.SaveTarget(controlRoot(), archive.Target{Name: "旧目标", Driver: "s3", Endpoint: "https://archive.example.invalid", Bucket: "archives", Prefix: "owned/host", Region: "us-east-1", CredentialRef: "owned", ForcePathStyle: true})
	require.NoError(t, e)
	require.NoError(t, s.BindArchiveControl(a))
	return &archiveTargetControl{s, a}, c, target, dir
}
func adoptNativeTarget(t *testing.T, c *archiveTargetControl, ctrl *ControlService) {
	t.Helper()
	out, e := c.Configuration(controlRoot())
	require.NoError(t, e)
	_, e = c.Adopt(controlRoot(), archive.TargetAdoption{RequestID: uuid.NewString(), LegacyDigest: out.LegacyDigest})
	require.NoError(t, e)
	applyMonitoring(t, ctrl, ctrl.opts.Queue)
}
func TestArchiveTargetNativePostgresAuthorityCASReceiptsAndLegacyPreservation(t *testing.T) {
	c, ctrl, target, dir := targetNative(t)
	ctx := controlRoot()
	before, e := os.ReadFile(filepath.Join(dir, "targets", target.ID+".json"))
	require.NoError(t, e)
	out, e := c.Configuration(ctx)
	require.NoError(t, e)
	require.Equal(t, "legacy", out.Authority)
	_, e = c.Resolve(ctx, target.ID)
	require.Equal(t, "ARCHIVE_TARGET_ADOPTION_REQUIRED", dto.CodeOf(e))
	_, e = c.archives.SaveTarget(ctx, target)
	require.Equal(t, "ARCHIVE_TARGET_CONTROL_REQUIRED", dto.CodeOf(e))
	key := uuid.NewString()
	adopt := archive.TargetAdoption{RequestID: key, LegacyDigest: out.LegacyDigest}
	saved, e := c.Adopt(ctx, adopt)
	require.NoError(t, e)
	require.Equal(t, "pending", saved.Configuration.State)
	_, e = c.Resolve(ctx, target.ID)
	require.Equal(t, "ARCHIVE_TARGET_PUBLICATION_PENDING", dto.CodeOf(e))
	applyMonitoring(t, ctrl, ctrl.opts.Queue)
	active, e := c.Resolve(ctx, target.ID)
	require.NoError(t, e)
	require.Equal(t, target.Revision+1, active.Revision)
	require.Nil(t, active.VerifiedAt)
	require.Len(t, active.ControlDigest, 64)
	in := archive.TargetChange{RequestID: uuid.NewString(), ExpectedRevision: 1, ExpectedTargetRevision: active.Revision, TargetID: target.ID, Settings: archive.SettingsOfTarget(active)}
	in.Settings.Prefix = "owned/changed"
	saved, e = c.Save(ctx, in)
	require.NoError(t, e)
	require.EqualValues(t, 2, saved.OperationRevision)
	_, e = c.Resolve(ctx, target.ID)
	require.Equal(t, "ARCHIVE_TARGET_PUBLICATION_PENDING", dto.CodeOf(e))
	called := false
	e = c.Confirm(ctx, active, func(archive.Target) error { called = true; return nil })
	require.Error(t, e)
	require.False(t, called, "stale network verification must not be recorded")
	replay, e := c.Save(ctx, in)
	require.NoError(t, e)
	require.EqualValues(t, 2, replay.OperationRevision)
	bad := in
	bad.Settings.Bucket = "different"
	_, e = c.Save(ctx, bad)
	require.Equal(t, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT", dto.CodeOf(e))
	applyMonitoring(t, ctrl, ctrl.opts.Queue)
	current, e := c.Resolve(ctx, target.ID)
	require.NoError(t, e)
	require.NotEqual(t, active.ControlDigest, current.ControlDigest)
	receipt, e := c.Request(ctx, key)
	require.NoError(t, e)
	require.EqualValues(t, 1, receipt.OperationRevision)
	require.EqualValues(t, 2, receipt.Configuration.DesiredRevision)
	replay, e = c.Adopt(ctx, adopt)
	require.NoError(t, e)
	require.EqualValues(t, 1, replay.OperationRevision)
	after, e := os.ReadFile(filepath.Join(dir, "targets", target.ID+".json"))
	require.NoError(t, e)
	require.Equal(t, before, after)
	var count int64
	require.NoError(t, ctrl.repo.DB.Model(&m.ControlRevision{}).Where("domain = ?", string(ControlArchiveTargets)).Count(&count).Error)
	require.EqualValues(t, 2, count)
}
func TestArchiveTargetNativePostgresProjectionTamperAndDatabaseRollbackBlock(t *testing.T) {
	c, ctrl, target, _ := targetNative(t)
	ctx := controlRoot()
	adoptNativeTarget(t, c, ctrl)
	raw, e := c.service.opts.Budget.Document(ctx, "archive-target-control", nil)
	require.NoError(t, e)
	var p archiveTargetProjection
	require.NoError(t, json.Unmarshal(raw, &p))
	p.Document = []byte(strings.Replace(string(p.Document), "owned/host", "owned/evil", 1))
	tampered, _ := json.Marshal(p)
	_, e = c.service.opts.Budget.Document(ctx, "archive-target-control", func([]byte) ([]byte, error) { return tampered, nil })
	require.NoError(t, e)
	_, e = c.Resolve(ctx, target.ID)
	require.Equal(t, "ARCHIVE_TARGET_SNAPSHOT_UNCONFIRMED", dto.CodeOf(e))
	_, e = c.service.opts.Budget.Document(ctx, "archive-target-control", func([]byte) ([]byte, error) { return raw, nil })
	require.NoError(t, e)
	require.NoError(t, ctrl.repo.DB.Where("domain = ?", string(ControlArchiveTargets)).Delete(&m.ControlConfiguration{}).Error)
	_, e = c.Resolve(ctx, target.ID)
	require.Equal(t, "ARCHIVE_TARGET_RESTORE_CONFIRMATION_REQUIRED", dto.CodeOf(e))
}
func TestArchiveTargetNativePostgresConcurrentCASAndUnchangedTarget(t *testing.T) {
	c, ctrl, target, _ := targetNative(t)
	ctx := controlRoot()
	adoptNativeTarget(t, c, ctrl)
	active, e := c.Resolve(ctx, target.ID)
	require.NoError(t, e)
	in := archive.TargetChange{RequestID: uuid.NewString(), ExpectedRevision: 1, Settings: archive.SettingsOfTarget(target)}
	in.Settings.Name = "独立新目标"
	_, e = c.Save(ctx, in)
	require.NoError(t, e)
	unchanged, e := c.Resolve(ctx, target.ID)
	require.NoError(t, e)
	require.Equal(t, active.ControlDigest, unchanged.ControlDigest, "unrelated publication must not invalidate target")
	applyMonitoring(t, ctrl, ctrl.opts.Queue)
	inputs := []archive.TargetChange{in, in}
	for i := range inputs {
		inputs[i].RequestID = uuid.NewString()
		inputs[i].ExpectedRevision = 2
		inputs[i].Settings.Name = "并发新目标"
	}
	errors := make([]error, 2)
	var wg sync.WaitGroup
	for i := range inputs {
		wg.Add(1)
		go func(i int) { defer wg.Done(); _, errors[i] = c.Save(ctx, inputs[i]) }(i)
	}
	wg.Wait()
	success, conflict := 0, 0
	for _, e := range errors {
		if e == nil {
			success++
		} else if dto.CodeOf(e) == "STORAGE_CONTROL_REVISION_CONFLICT" {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)
	_, e = c.Request(ctx, uuid.NewString())
	require.Equal(t, "STORAGE_CONTROL_REQUEST_NOT_FOUND", dto.CodeOf(e))
}
func TestArchiveTargetNativePostgresSourceDriftAndCredentialFieldsRejected(t *testing.T) {
	c, ctrl, target, dir := targetNative(t)
	ctx := controlRoot()
	out, e := c.Configuration(ctx)
	require.NoError(t, e)
	path := filepath.Join(dir, "targets", target.ID+".json")
	before, e := os.ReadFile(path)
	require.NoError(t, e)
	var changed archive.Target
	require.NoError(t, json.Unmarshal(before, &changed))
	changed.Name = "现场变更"
	raw, _ := json.Marshal(changed)
	require.NoError(t, os.WriteFile(path, raw, 0600))
	_, e = c.Adopt(ctx, archive.TargetAdoption{RequestID: uuid.NewString(), LegacyDigest: out.LegacyDigest})
	require.Equal(t, "ARCHIVE_TARGET_LEGACY_CHANGED", dto.CodeOf(e))
	require.NoError(t, os.WriteFile(path, before, 0600))
	adoptNativeTarget(t, c, ctrl)
	state, e := ctrl.read(ctx, ControlArchiveTargets)
	require.NoError(t, e)
	raw = []byte(strings.Replace(string(state.Document), `"settings":{`, `"settings":{"secret_key":"forbidden",`, 1))
	_, e = ctrl.Save(ctx, ControlInput{uuid.NewString(), 1, ControlArchiveTargets, raw})
	require.Error(t, e)
}

func TestArchiveTargetNativePostgresSupersededAdoptionStillChecksOrigin(t *testing.T) {
	c, ctrl, target, dir := targetNative(t)
	ctx := controlRoot()
	configuration, e := c.Configuration(ctx)
	require.NoError(t, e)
	_, e = c.Adopt(ctx, archive.TargetAdoption{RequestID: uuid.NewString(), LegacyDigest: configuration.LegacyDigest})
	require.NoError(t, e)
	in := archive.TargetChange{RequestID: uuid.NewString(), ExpectedRevision: 1, ExpectedTargetRevision: target.Revision + 1, TargetID: target.ID, Settings: archive.SettingsOfTarget(target)}
	in.Settings.Name = "首次发布前编辑"
	_, e = c.Save(ctx, in)
	require.NoError(t, e)
	target.Name = "旧来源在提交后被外部修改"
	raw, e := json.Marshal(target)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "targets", target.ID+".json"), raw, 0600))
	items, e := ctrl.opts.Queue.Dequeue(ctx, controlQueueTenant, ctrl.subscriber(), "owned-target", "owned-worker", 10, time.Millisecond)
	require.NoError(t, e)
	require.NotEmpty(t, items)
	failed := false
	for _, item := range items {
		if ctrl.ApplyDelivery(ctx, item) != nil {
			failed = true
		}
	}
	require.True(t, failed, "newer revision cannot bypass initial origin verification")
	state, e := ctrl.read(ctx, ControlArchiveTargets)
	require.NoError(t, e)
	require.Zero(t, state.AppliedRevision)
	require.Equal(t, "failed", state.State)
	_, e = c.Resolve(ctx, target.ID)
	require.Error(t, e)
}
