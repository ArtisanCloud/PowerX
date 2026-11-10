package storagegov

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	mevent "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func controlRoot() context.Context {
	return reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 7, Scope: "access"})
}
func controlBinding(apply func(context.Context, ControlPublication) error) ControlBinding {
	return ControlBinding{Validate: func(body json.RawMessage) error {
		var value struct {
			Days    int  `json:"days"`
			Enabled bool `json:"enabled"`
		}
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.DisallowUnknownFields()
		if dec.Decode(&value) != nil || value.Days < 1 || value.Days > 3650 {
			return errors.New("invalid owned policy")
		}
		return nil
	}, Apply: apply}
}
func ownedControl(t *testing.T, tasks bool) (*gorm.DB, *ControlService, *atomic.Int32) {
	t.Helper()
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	require.NoError(t, db.AutoMigrate(&m.ControlConfiguration{}, &m.ControlRevision{}))
	if tasks {
		require.NoError(t, db.AutoMigrate(&mevent.TaskHistory{}))
	}
	count := &atomic.Int32{}
	svc, e := NewControlService(db, ControlOptions{Scope: ControlScope{HostID: "owned-host", FilesystemID: "owned-fs", InstanceID: "owned-install"}, Queue: taskqueue.NewStreamDriver(db, nil), Dispatch: func(context.Context, event_bus.TaskMessage) error {
		count.Add(1)
		return errors.New("owned transport unavailable")
	}})
	require.NoError(t, e)
	require.NoError(t, svc.Bind(ControlLifecycle, controlBinding(func(context.Context, ControlPublication) error { return nil })))
	return db, svc, count
}
func controlInput(revision int64, days int) ControlInput {
	body, _ := json.Marshal(map[string]any{"days": days, "enabled": false})
	return ControlInput{Domain: ControlLifecycle, ExpectedRevision: revision, RequestID: uuid.NewString(), Document: body}
}

func TestControlCommitIsAtomicWithTaskBusAndDoesNotClaimApplied(t *testing.T) {
	db, s, dispatch := ownedControl(t, true)
	ctx := controlRoot()
	input := controlInput(0, 7)
	out, e := s.Save(ctx, input)
	require.NoError(t, e)
	require.EqualValues(t, 1, out.DesiredRevision)
	require.EqualValues(t, 0, out.AppliedRevision)
	require.Equal(t, "pending", out.State)
	require.EqualValues(t, 1, dispatch.Load())
	for _, model := range []any{&m.ControlConfiguration{}, &m.ControlRevision{}, &mevent.TaskHistory{}} {
		var count int64
		require.NoError(t, db.Model(model).Count(&count).Error)
		require.EqualValues(t, 1, count)
	}
	var task mevent.TaskHistory
	require.NoError(t, db.Select("status", "topic").First(&task).Error)
	require.Equal(t, mevent.TaskHistoryStatusPending, task.Status)
	require.Equal(t, controlTopic, task.Topic)
}
func TestControlMissingTaskHistoryRollsBackConfigAuditAndReceipt(t *testing.T) {
	db, s, dispatch := ownedControl(t, false)
	_, e := s.Save(controlRoot(), controlInput(0, 7))
	require.Equal(t, "STORAGE_CONTROL_SAVE_UNAVAILABLE", dto.CodeOf(e))
	require.Zero(t, dispatch.Load())
	for _, model := range []any{&m.ControlConfiguration{}, &m.ControlRevision{}} {
		var count int64
		require.NoError(t, db.Model(model).Count(&count).Error)
		require.Zero(t, count)
	}
}
func TestControlConcurrentCASHasOneWinnerAndOneAudit(t *testing.T) {
	db, s, _ := ownedControl(t, true)
	ctx := controlRoot()
	_, e := s.Save(ctx, controlInput(0, 7))
	require.NoError(t, e)
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, days := range []int{14, 30} {
		workers.Add(1)
		go func(days int) {
			defer workers.Done()
			<-start
			_, e := s.Save(ctx, controlInput(1, days))
			results <- e
		}(days)
	}
	close(start)
	workers.Wait()
	close(results)
	success, conflicts := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else {
			require.Equal(t, "STORAGE_CONTROL_REVISION_CONFLICT", dto.CodeOf(e))
			conflicts++
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflicts)
	var count int64
	require.NoError(t, db.Model(&m.ControlRevision{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
}
func TestControlIdempotencyReturnsReceiptAndRejectsDifferentPayloadOrActor(t *testing.T) {
	db, s, dispatch := ownedControl(t, true)
	ctx := controlRoot()
	input := controlInput(0, 7)
	_, e := s.Save(ctx, input)
	require.NoError(t, e)
	input.Document = json.RawMessage(` { "enabled":false, "days":7 } `)
	out, e := s.Save(ctx, input)
	require.NoError(t, e)
	require.True(t, out.Replayed)
	require.EqualValues(t, 1, out.OperationRevision)
	require.EqualValues(t, 1, dispatch.Load())
	_, e = s.Save(ctx, controlInput(1, 14))
	require.NoError(t, e)
	out, e = s.Save(ctx, input)
	require.NoError(t, e)
	require.True(t, out.Replayed)
	require.EqualValues(t, 1, out.OperationRevision, "old request keeps its original receipt")
	require.EqualValues(t, 2, out.DesiredRevision, "receipt is separate from the latest configuration")
	input.Document = json.RawMessage(`{"days":8,"enabled":false}`)
	_, e = s.Save(ctx, input)
	require.Equal(t, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT", dto.CodeOf(e))
	input.Document = json.RawMessage(`{"days":7,"enabled":false}`)
	other := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: true, UserID: 8, Scope: "access"})
	_, e = s.Save(other, input)
	require.Equal(t, "STORAGE_CONTROL_IDEMPOTENCY_CONFLICT", dto.CodeOf(e))
	var count int64
	require.NoError(t, db.Model(&m.ControlRevision{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
}
func TestControlSupersededTaskCannotPublishAndApplyFailureCannotAdvanceVersion(t *testing.T) {
	db, s, _ := ownedControl(t, true)
	ctx := controlRoot()
	_, e := s.Save(ctx, controlInput(0, 7))
	require.NoError(t, e)
	_, e = s.Save(ctx, controlInput(1, 14))
	require.NoError(t, e)
	r := repo.NewControlRepository(db)
	called := 0
	for _, version := range []int64{1, 2} {
		row, e := r.Revision(ctx, s.opts.Scope.ID(), string(ControlLifecycle), version)
		require.NoError(t, e)
		e = db.Transaction(func(tx *gorm.DB) error {
			applied, e := r.Apply(ctx, tx, s.opts.Scope.ID(), row.Domain, row.Revision, row.Digest, func(m.ControlConfiguration) error { called++; return errors.New("owned projection failure") })
			require.False(t, applied)
			return e
		})
		if version == 1 {
			require.NoError(t, e)
			require.Zero(t, called)
		} else {
			require.Error(t, e)
			require.Equal(t, 1, called)
		}
	}
	out, e := s.Read(ctx, ControlLifecycle)
	require.NoError(t, e)
	require.Zero(t, out.AppliedRevision)
	require.Equal(t, "pending", out.State)
	row, e := r.Revision(ctx, s.opts.Scope.ID(), string(ControlLifecycle), 2)
	require.NoError(t, e)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		applied, e := r.Apply(ctx, tx, s.opts.Scope.ID(), row.Domain, 2, row.Digest, func(m.ControlConfiguration) error { return nil })
		require.True(t, applied)
		return e
	}))
	out, e = s.Read(ctx, ControlLifecycle)
	require.NoError(t, e)
	require.EqualValues(t, 2, out.AppliedRevision)
	require.Equal(t, "applied", out.State)
}
func TestControlScopeAndSecretBoundaries(t *testing.T) {
	db, s, _ := ownedControl(t, true)
	ctx := controlRoot()
	for _, body := range []string{`{"days":7,"secret_key":"owned-test"}`, `{"days":7,"SecretKey":"owned-test"}`, `{"days":7,"nested":{"password":"owned-test"}}`} {
		input := controlInput(0, 7)
		input.Document = json.RawMessage(body)
		_, e := s.Save(ctx, input)
		require.Equal(t, "STORAGE_CONTROL_SECRET_FORBIDDEN", dto.CodeOf(e))
	}
	input := controlInput(0, 7)
	input.Domain = "shell"
	_, e := s.Save(ctx, input)
	require.Equal(t, "STORAGE_CONTROL_DOMAIN_INVALID", dto.CodeOf(e))
	_, e = s.Save(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{IsRoot: false, UserID: 9, Scope: "access"}), controlInput(0, 7))
	require.ErrorIs(t, e, ErrRootRequired)
	_, e = s.Save(ctx, controlInput(0, 7))
	require.NoError(t, e)
	other, e := NewControlService(db, ControlOptions{Scope: ControlScope{HostID: "different-host", FilesystemID: "owned-fs", InstanceID: "owned-install"}, Queue: s.opts.Queue, Dispatch: s.opts.Dispatch})
	require.NoError(t, e)
	out, e := other.Read(ctx, ControlLifecycle)
	require.NoError(t, e)
	require.Equal(t, "unconfigured", out.State)
}

func TestControlCanonicalDigestSurvivesJSONBFormattingAndNumbers(t *testing.T) {
	a, e := canonicalControl([]byte(`{"days":7,"fraction":0.10,"large":1e6,"zero":-0.0}`))
	require.NoError(t, e)
	b, e := canonicalControl([]byte(`{ "zero":0, "large":1000000.0,"fraction":0.1,"days":7.00}`))
	require.NoError(t, e)
	require.Equal(t, string(a), string(b))
	_, e = verifiedControlDocument(b, controlDigest(a))
	require.NoError(t, e)
	_, e = verifiedControlDocument([]byte(`{"days":8}`), controlDigest(a))
	require.Equal(t, "STORAGE_CONTROL_DIGEST_MISMATCH", dto.CodeOf(e))
	for _, input := range []string{`{"days":1e999999999}`, `{"days":1e-2000}`, `{"days":1e1000}`} {
		_, e = canonicalControl([]byte(input))
		require.Equal(t, "STORAGE_CONTROL_NUMBER_INVALID", dto.CodeOf(e))
	}
}
