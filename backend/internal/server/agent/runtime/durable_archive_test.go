package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	chatmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	sessions "github.com/ArtisanCloud/PowerX/internal/service/agent_session"
	trace "github.com/ArtisanCloud/PowerX/internal/service/agent_trace"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type unavailableArchiveObjects struct {
	agent_run.ReportObjectStore
	unavailable bool
	puts        int
}

func (s *unavailableArchiveObjects) Put(ctx context.Context, key string, body []byte) error {
	s.puts++
	if s.unavailable {
		return errors.New("archive temporarily unavailable")
	}
	return s.ReportObjectStore.Put(ctx, key, body)
}

func TestTerminalArchiveRecoversObjectAndLocatorFailuresWithoutNewMessages(t *testing.T) {
	var client *redis.Client
	admin, db, ctx, cfg := adminRunFixture(t, func(c *redis.Client) { client = c })
	run, err := admin.Admit(ctx, "review", cfg, "", "archive-review")
	require.NoError(t, err)
	run, err = admin.Store.Cancel(ctx, run)
	require.NoError(t, err)
	objects := &unavailableArchiveObjects{ReportObjectStore: admin.Objects, unavailable: true}
	consumers := NewDurableConsumers(sessions.NewService(db), nil, admin)
	require.NoError(t, consumers.ConfigureArchive(admin.Store, objects))
	require.ErrorContains(t, consumers.FinalizeRun(ctx, run), "archive temporarily unavailable")
	var anchor model.AdminRunAdmission
	require.NoError(t, db.First(&anchor).Error)
	require.NotNil(t, anchor.FinishedAt)
	require.Nil(t, anchor.ArchivedAt)
	rows, err := admin.ListUnfinishedRuns(ctx, 0, 200)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	eventsBefore, err := admin.Store.Events(ctx, run.TenantUUID, run.Env, run.RunID, 0, 1000)
	require.NoError(t, err)
	objects.unavailable = false
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_archive BEFORE UPDATE OF archive_key ON agent_admin_run_admissions BEGIN SELECT RAISE(ABORT, 'archive locator unavailable'); END`).Error)
	require.ErrorContains(t, consumers.FinalizeRun(ctx, run), "archive locator unavailable")
	stored, err := admin.Store.Get(ctx, run.TenantUUID, run.Env, run.RunID)
	require.NoError(t, err)
	require.NotEmpty(t, stored.ArchiveKey)
	require.NoError(t, db.Exec(`DROP TRIGGER reject_archive`).Error)
	writes := objects.puts
	for i := 0; i < 3; i++ {
		require.NoError(t, consumers.FinalizeRun(ctx, run))
	}
	require.Equal(t, writes, objects.puts, "SQL retry must reuse verified object")
	require.NoError(t, db.First(&anchor).Error)
	require.Equal(t, stored.ArchiveKey, anchor.ArchiveKey)
	require.NotNil(t, anchor.ArchivedAt)
	rows, err = admin.ListUnfinishedRuns(ctx, 0, 200)
	require.NoError(t, err)
	require.Empty(t, rows)
	var count int64
	require.NoError(t, db.Model(&chatmodel.AgentChatMessage{}).Where("role = ?", "assistant").Count(&count).Error)
	require.EqualValues(t, 1, count)
	report, err := agent_run.ReadRunReport(ctx, run, objects)
	require.NoError(t, err)
	require.Equal(t, eventsBefore, report.Events)
	require.NotEmpty(t, report.Tasks, "planning task must survive archiving")
	wrong := run
	wrong.MessageID = uuid.NewString()
	_, err = admin.Store.ArchiveRun(ctx, wrong, objects)
	require.ErrorIs(t, err, agent_run.ErrConflict)
	_, err = agent_run.ReadRunReport(ctx, wrong, objects)
	require.ErrorIs(t, err, agent_run.ErrInvalid)
	wrong = run
	wrong.TenantUUID = uuid.NewString()
	require.Error(t, consumers.FinalizeRun(ctx, wrong))
	// 仅清空本测试独占的 miniredis，模拟热状态保留期结束。
	require.NoError(t, client.FlushDB(ctx).Err())
	query := trace.AgentReportQuery{TenantUUID: run.TenantUUID, SessionID: run.SessionID, MessageID: run.MessageID, RunID: run.RunID}
	restored, err := admin.BuildReport(ctx, query)
	require.NoError(t, err)
	require.Equal(t, "object_storage", restored.Summary["state_backend"])
	require.Equal(t, "cancelled", restored.Summary["status"])
	require.Len(t, restored.Timeline, len(eventsBefore))
	require.NoError(t, admin.ValidateSubscription(ctx, run.RunID, 0))
	require.ErrorIs(t, admin.ValidateSubscription(ctx, run.RunID, run.EventSeq+1), agent_run.ErrInvalid)
	finals, durableEvents := 0, 0
	require.NoError(t, admin.Subscribe(ctx, run.RunID, 0, func(seq uint64, event string, _ any) error {
		if seq > 0 {
			durableEvents++
		}
		if event == dto.EventFinal {
			finals++
		}
		return nil
	}))
	require.Equal(t, 1, finals)
	require.Equal(t, len(eventsBefore), durableEvents)

	_, err = admin.BuildReport(reqctx.WithUserUUID(ctx, uuid.NewString()), query)
	require.Error(t, err)
	// 归档损坏不得回退到本地日志或泄露错误租户的数据。
	underlying := admin.Objects.(*planMemoryObjects)
	underlying.mu.Lock()
	underlying.items[stored.ArchiveKey] = []byte(`{"report":{}}`)
	underlying.mu.Unlock()
	_, err = admin.BuildReport(ctx, query)
	require.Error(t, err)
}

func TestArchiveLifecycleBackpressureAndRetentionRecovery(t *testing.T) {
	var client *redis.Client
	admin, db, ctx, cfg := adminRunFixture(t, func(c *redis.Client) { client = c })
	require.NoError(t, db.AutoMigrate(&model.ServiceSession{}, &model.ServiceMessage{}, &model.ServiceInvocation{}))
	service := sessions.NewService(db)
	require.NoError(t, service.ConfigureDurableAdmission(admin.Store, admin.Queue, "dev", time.Hour))
	consumers := NewDurableConsumers(service, nil, admin)
	consumers.ConfigureArchiveQueue(admin.Queue.(agent_run.ArchivedTaskQueue))
	objects := &unavailableArchiveObjects{ReportObjectStore: admin.Objects}
	require.NoError(t, consumers.ConfigureArchive(admin.Store, objects))
	policy := agent_run.ArchivePolicy{HotRetention: time.Hour, MaxPending: 1, MaxAge: time.Minute, HealthTTL: 15 * time.Second}
	require.NoError(t, consumers.ConfigureArchiveLifecycle("dev", policy))
	require.NoError(t, consumers.MaintainRuns(ctx))
	require.NoError(t, admin.Store.CheckArchiveAdmission(ctx, "dev", policy))
	run, err := admin.Admit(ctx, "review", cfg, "", "lifecycle-review")
	require.NoError(t, err)
	run, err = admin.Store.Cancel(ctx, run)
	require.NoError(t, err)
	require.NoError(t, admin.FinalizeRun(ctx, run))
	require.NoError(t, consumers.MaintainRuns(ctx))
	require.ErrorIs(t, admin.Store.CheckArchiveAdmission(ctx, "dev", policy), agent_run.ErrArchiveBackpressure)
	_, err = admin.Admit(ctx, "review", cfg, "", "new-request")
	require.ErrorIs(t, err, agent_run.ErrArchiveBackpressure)
	replay, err := admin.Admit(ctx, "review", cfg, "", "lifecycle-review")
	require.NoError(t, err)
	require.Equal(t, run.RunID, replay.RunID, "backpressure must allow existing Run recovery")
	objects.unavailable = true
	require.Error(t, consumers.FinalizeRun(ctx, run))
	keys, err := client.Keys(ctx, "agent:run:*").Result()
	require.NoError(t, err)
	for _, key := range keys {
		require.Equal(t, time.Duration(-1), client.TTL(ctx, key).Val(), "failed archive must not expire")
	}
	objects.unavailable = false
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_hot_expiry BEFORE UPDATE OF hot_expires_at ON agent_admin_run_admissions BEGIN SELECT RAISE(ABORT, 'expiry locator unavailable'); END`).Error)
	require.ErrorContains(t, consumers.FinalizeRun(ctx, run), "expiry locator unavailable")
	rows, err := admin.ListUnfinishedRuns(ctx, 0, 200)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	var anchor model.AdminRunAdmission
	require.NoError(t, db.First(&anchor).Error)
	require.NotNil(t, anchor.ArchivedAt)
	require.Nil(t, anchor.HotExpiresAt)
	require.NoError(t, db.Exec(`DROP TRIGGER reject_hot_expiry`).Error)
	require.NoError(t, consumers.FinalizeRun(ctx, run))
	require.NoError(t, db.First(&anchor).Error)
	require.NotNil(t, anchor.HotExpiresAt)
	require.WithinDuration(t, anchor.ArchivedAt.Add(time.Hour), *anchor.HotExpiresAt, time.Millisecond)
	rows, err = admin.ListUnfinishedRuns(ctx, 0, 200)
	require.NoError(t, err)
	require.Empty(t, rows)
	keys, err = client.Keys(ctx, "agent:run:*").Result()
	require.NoError(t, err)
	require.NotEmpty(t, keys)
	for _, key := range keys {
		require.Greater(t, client.TTL(ctx, key).Val(), time.Duration(0), key)
	}
	for _, key := range client.Keys(ctx, "event_fabric:task:*:stream").Val() {
		require.Zero(t, client.XLen(ctx, key).Val(), "terminal planning delivery must be removed")
	}
	require.NoError(t, consumers.MaintainRuns(ctx))
	require.NoError(t, admin.Store.CheckArchiveAdmission(ctx, "dev", policy))
	var count int64
	require.NoError(t, db.Model(&chatmodel.AgentChatMessage{}).Where("role = ?", "assistant").Count(&count).Error)
	require.EqualValues(t, 1, count)
}
