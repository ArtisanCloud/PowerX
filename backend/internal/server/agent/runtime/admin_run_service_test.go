package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	chatmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentauthz "github.com/ArtisanCloud/PowerX/internal/service/agent_authz"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	trace "github.com/ArtisanCloud/PowerX/internal/service/agent_trace"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func adminRunFixture(t *testing.T, observeRedis ...func(*redis.Client)) (*AdminRunService, *gorm.DB, context.Context, *dto.ChatConfig) {
	t.Helper()
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.AdminRunAdmission{}, &iam.User{}, &iam.Member{}))
	// 旧聊天模型含 PostgreSQL JSONB 默认表达式；此处使用等价 SQLite 表测试真实事务路径。
	require.NoError(t, db.Exec(fmt.Sprintf(`CREATE TABLE %s (id integer primary key, uuid text unique, created_at datetime, updated_at datetime, deleted_at datetime, env text, tenant_uuid text, agent_id integer, user_id integer, title text, singleton boolean, ttl_days integer, max_kb integer, max_tokens integer, summary text, summary_at datetime, status text, latest_at datetime, expired_at datetime, meta text)`, chatmodel.TableAgentChatSession)).Error)
	require.NoError(t, db.Exec(fmt.Sprintf(`CREATE TABLE %s (id integer primary key, uuid text unique, created_at datetime, updated_at datetime, deleted_at datetime, env text, tenant_uuid text, session_id integer, agent_id integer, role text, content text, content_type text, format text, tokens integer, size_bytes integer, pinned boolean, is_error boolean, meta text)`, chatmodel.TableAgentChatMessage)).Error)
	actor := iam.User{Status: 1, IsRoot: true}
	require.NoError(t, db.Create(&actor).Error)
	tenant := uuid.NewString()
	session := chatmodel.AgentChatSession{Env: "dev", TenantUUID: &tenant, AgentID: 7, UserID: actor.ID, Status: "active"}
	require.NoError(t, db.Create(&session).Error)
	message := chatmodel.AgentChatMessage{Env: "dev", TenantUUID: &tenant, AgentID: 7, SessionID: session.ID, Role: "user", Content: "review"}
	require.NoError(t, db.Create(&message).Error)
	snapshot, err := NewResourceSnapshot(tenant, uuid.New(), nil, time.Now())
	require.NoError(t, err)
	snapshot.ExpiresAt = time.Now().Add(time.Hour)
	ctx := reqctx.WithEnv(reqctx.WithTraceID(reqctx.WithUserUUID(reqctx.WithUserID(reqctx.WithTenantUUID(context.Background(), tenant), actor.ID), actor.UUID.String()), uuid.NewString()), "dev")
	for key, value := range map[string]any{"session_uuid": session.UUID.String(), "message_uuid": message.UUID.String(), "runtime_run_uuid": uuid.NewString(), "agent_bound_skill_ids": []string{"skill.review"}, "authorization": "do-not-persist-bearer"} {
		ctx = context.WithValue(ctx, key, value)
	}
	ctx = ContextWithResourceSnapshot(ctx, snapshot)
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	for _, observe := range observeRedis {
		observe(client)
	}
	t.Cleanup(func() { _ = client.Close() })
	store, err := agent_run.NewRedisStore(client)
	require.NoError(t, err)
	queue, err := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	require.NoError(t, err)
	objects := &planMemoryObjects{items: map[string][]byte{}}
	service := NewAdminRunService(db, store, queue, objects, "dev", time.Hour)
	return service, db, ctx, &dto.ChatConfig{Provider: "test", ModelName: "model", APIKey: "do-not-persist-api-key", SystemPrompt: "frozen instructions"}
}

func TestAdminRunAdmissionReplayAndSingleTerminalMessage(t *testing.T) {
	service, db, ctx, cfg := adminRunFixture(t)
	run, err := service.Admit(ctx, "review", cfg, "", "request-key")
	require.NoError(t, err)
	again, err := service.Admit(ctx, "review", cfg, "", "request-key")
	require.NoError(t, err)
	require.Equal(t, run.RunID, again.RunID)
	_, err = service.Admit(ctx, "review", cfg, "", "other-key")
	require.ErrorIs(t, err, agent_run.ErrConflict)
	var anchor model.AdminRunAdmission
	require.NoError(t, db.First(&anchor).Error)
	raw, err := service.Objects.Get(ctx, anchor.InputRef)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "do-not-persist")
	require.NotContains(t, string(raw), "authorization")
	input, err := loadAdminRunInput(ctx, service.Objects, &anchor)
	require.NoError(t, err)
	require.Equal(t, "frozen instructions", input.Config.SystemPrompt)
	_, err = service.Store.Cancel(ctx, run)
	require.NoError(t, err)
	// 模拟 Worker 已写终态、尚未回写消息即退出；订阅和扫描器均能幂等补写。
	for i := 0; i < 2; i++ {
		count := 0
		require.NoError(t, service.Subscribe(ctx, run.RunID, 0, func(_ uint64, event string, payload any) error {
			if event == dto.EventFinal {
				count++
				raw, err := json.Marshal(payload)
				require.NoError(t, err)
				require.Contains(t, string(raw), `"success":false`)
			}
			return nil
		}))
		require.Equal(t, 1, count)
	}
	var messages []chatmodel.AgentChatMessage
	require.NoError(t, db.Where("role = ?", "assistant").Find(&messages).Error)
	require.Len(t, messages, 1)
	require.Equal(t, run.RunID, messages[0].Meta["run_id"])
	require.NoError(t, service.FinalizeRun(ctx, run))
	locators, err := service.ListUnfinishedRuns(ctx, 0, 200)
	require.NoError(t, err)
	require.Empty(t, locators)
	query := trace.AgentReportQuery{TenantUUID: run.TenantUUID, SessionID: anchor.SessionUUID.String(), MessageID: anchor.MessageUUID.String(), RunID: run.RunID}
	report, err := service.BuildReport(ctx, query)
	require.NoError(t, err)
	require.Equal(t, "redis", report.Summary["state_backend"])
	require.Equal(t, "cancelled", report.Summary["status"])
	require.NotEmpty(t, report.Timeline)
	query.MessageID = uuid.NewString()
	_, err = service.BuildReport(ctx, query)
	require.ErrorIs(t, err, agent_run.ErrInvalid)
	_, err = service.AuthorizedRun(reqctx.WithUserUUID(ctx, uuid.NewString()), run.RunID)
	require.Error(t, err)
	_, err = service.AuthorizedRun(reqctx.WithTenantUUID(ctx, uuid.NewString()), run.RunID)
	require.Error(t, err)
	require.NoError(t, db.Model(&iam.User{}).Where("uuid = ?", reqctx.GetUserUUID(ctx)).Update("status", 2).Error)
	_, err = service.AuthorizedRun(ctx, run.RunID)
	require.Error(t, err)
}

func TestAdminInputArtifactRejectsTamperingAndCredentialEndpoint(t *testing.T) {
	service, db, ctx, cfg := adminRunFixture(t)
	cfg.Endpoint = "https://model.example/api?key=secret"
	_, err := service.Admit(ctx, "review", cfg, "", "request")
	require.ErrorContains(t, err, "inline credentials")
	cfg.Endpoint = "https://model.example/api"
	_, err = service.Admit(ctx, "review", cfg, "", "request")
	require.NoError(t, err)
	var anchor model.AdminRunAdmission
	require.NoError(t, db.First(&anchor).Error)
	require.NoError(t, service.Objects.Put(ctx, anchor.InputRef, []byte(`{"tampered":true}`)))
	_, err = loadAdminRunInput(ctx, service.Objects, &anchor)
	require.ErrorIs(t, err, agent_run.ErrConflict)
}

type adminTestPermissions struct {
	allowed bool
	calls   int
}

func (a *adminTestPermissions) ResolveEffectivePermissions(ctx context.Context, env, tenant, user, member string, id uint64, root bool, agent uuid.UUID) (agentauthz.EffectivePermissionsResult, error) {
	a.calls++
	if !root || reqctx.GetTenantUUID(ctx) != tenant || reqctx.GetUserUUID(ctx) != user {
		return agentauthz.EffectivePermissionsResult{}, fmt.Errorf("actor context missing")
	}
	return agentauthz.EffectivePermissionsResult{AgentAccessAllowed: a.allowed}, nil
}

type adminTestSnapshot struct{ snapshot *ResourceSnapshot }

func (r adminTestSnapshot) Restore(context.Context, string, string, uuid.UUID, uuid.UUID) (*ResourceSnapshot, error) {
	return r.snapshot, nil
}

type adminTestConfig struct{ config *dto.ChatConfig }

func (r adminTestConfig) ResolveForAgentChat(context.Context, string, *string, uint64, *dto.ChatConfig) (*dto.ChatConfig, error) {
	return r.config, nil
}

func TestAdminWorkerReloadsCredentialsAndRechecksAuthorization(t *testing.T) {
	service, db, ctx, cfg := adminRunFixture(t)
	snapshot, _ := ResourceSnapshotFromContext(ctx)
	require.NoError(t, db.Exec(`CREATE TABLE agents (id integer primary key, uuid text, env text, tenant_uuid text, status text, deleted_at datetime)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO agents (id,uuid,env,tenant_uuid,status) VALUES (7,?,?,?,'active')`, snapshot.AgentUUID, "dev", snapshot.TenantUUID).Error)
	require.NoError(t, db.AutoMigrate(&chatmodel.AgentSkillBinding{}))
	require.NoError(t, db.Create(&chatmodel.AgentSkillBinding{Env: "dev", TenantUUID: &snapshot.TenantUUID, AgentID: 7, SkillID: "skill.review"}).Error)
	permissions := &adminTestPermissions{allowed: true}
	service.Loader.authz = permissions
	service.Loader.snapshots = adminTestSnapshot{snapshot}
	live := *cfg
	live.APIKey = "rotated-key"
	live.SystemPrompt = "changed-system-prompt"
	live.MaxConcurrentRequests = 1
	service.Loader.config = adminTestConfig{&live}
	run, err := service.Admit(ctx, "review", cfg, "", "request")
	require.NoError(t, err)
	ref := agent_run.TaskRef{TenantUUID: run.TenantUUID, Env: run.Env, RunID: run.RunID, TaskID: "plan", Attempt: 1}
	restored, err := service.Loader.Load(context.Background(), ref)
	require.NoError(t, err)
	require.Equal(t, "rotated-key", restored.Config.APIKey)
	require.Equal(t, "frozen instructions", restored.Config.SystemPrompt)
	require.Equal(t, 1, restored.Config.MaxConcurrentRequests)
	require.Nil(t, restored.Context.Value("authorization"))
	permissions.allowed = false
	_, err = service.Loader.Load(context.Background(), ref)
	require.ErrorContains(t, err, "access revoked")
	require.Equal(t, 2, permissions.calls)
	permissions.allowed = true
	live.ModelName = "different-model"
	_, err = service.Loader.Load(context.Background(), ref)
	require.ErrorContains(t, err, "configuration changed")
}

func TestAdminAdmissionCannotOutliveAuthorizationSnapshot(t *testing.T) {
	service, _, ctx, cfg := adminRunFixture(t)
	snapshot, _ := ResourceSnapshotFromContext(ctx)
	snapshot.ExpiresAt = time.Now().Add(2 * time.Minute).Truncate(time.Microsecond)
	run, err := service.Admit(ctx, "review", cfg, "", "request")
	require.NoError(t, err)
	require.True(t, run.DeadlineAt.Equal(snapshot.ExpiresAt))
}

func TestAdminPendingAdmissionRecoveredWithoutRebuildingAdmittedRun(t *testing.T) {
	service, db, ctx, cfg := adminRunFixture(t)
	run, err := service.Admit(ctx, "review", cfg, "", "request")
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.AdminRunAdmission{}).Where("uuid = ?", run.RunID).Update("admission_state", "pending_create").Error)
	rows, err := service.ListUnfinishedRuns(ctx, 0, 200)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	var anchor model.AdminRunAdmission
	require.NoError(t, db.First(&anchor).Error)
	require.Equal(t, "admitted", anchor.AdmissionState)
	current, err := service.Store.Get(ctx, run.TenantUUID, run.Env, run.RunID)
	require.NoError(t, err)
	require.Equal(t, run.Version, current.Version)
	require.NoError(t, service.ValidateSubscription(ctx, run.RunID, current.EventSeq))
	require.ErrorIs(t, service.ValidateSubscription(ctx, run.RunID, current.EventSeq+1), agent_run.ErrInvalid)
}

func TestAdminExplicitRetryCreatesNewRunForSameMessage(t *testing.T) {
	service, db, ctx, cfg := adminRunFixture(t)
	first, err := service.Admit(ctx, "review", cfg, "", "retry-first")
	require.NoError(t, err)
	cancelled, err := service.Store.Cancel(ctx, first)
	require.NoError(t, err)
	require.NoError(t, service.FinalizeRun(ctx, cancelled))
	snapshot, ok := ResourceSnapshotFromContext(ctx)
	require.True(t, ok)
	nextSnapshot := *snapshot
	nextSnapshot.SnapshotUUID = uuid.New()
	nextCtx := ContextWithResourceSnapshot(context.WithValue(ctx, "runtime_run_uuid", uuid.NewString()), &nextSnapshot)
	repeated, err := service.Admit(nextCtx, "review", cfg, "", "retry-first")
	require.NoError(t, err)
	require.Equal(t, first.RunID, repeated.RunID, "同一次受理的重复请求保持幂等")
	next, err := service.Admit(nextCtx, "review", cfg, "", "retry-second")
	require.NoError(t, err)
	require.NotEqual(t, first.RunID, next.RunID, "主动重试必须得到新的 Run")
	require.Equal(t, first.MessageID, next.MessageID)
	repeated, err = service.Admit(nextCtx, "review", cfg, "", "retry-second")
	require.NoError(t, err)
	require.Equal(t, next.RunID, repeated.RunID)
	var count int64
	require.NoError(t, db.Model(&model.AdminRunAdmission{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
	require.NoError(t, db.Model(&chatmodel.AgentChatMessage{}).Where("role = ?", "user").Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestAdminFailedHistoricalRunEndsWhenAssistantWasTruncated(t *testing.T) {
	service, db, ctx, cfg := adminRunFixture(t)
	run, err := service.Admit(ctx, "review", cfg, "", "historical")
	require.NoError(t, err)
	terminal, err := service.Store.Transition(ctx, run, run.Version, "failed", "agent_run.final")
	require.NoError(t, err)
	require.NoError(t, service.FinalizeRun(ctx, terminal))
	var row model.AdminRunAdmission
	require.NoError(t, db.First(&row).Error)
	require.NoError(t, db.Where("uuid = ?", *row.AssistantMessageUUID).Delete(&chatmodel.AgentChatMessage{}).Error)
	for _, cursor := range []uint64{0, terminal.EventSeq} {
		var ended, end int
		require.NoError(t, service.Subscribe(ctx, run.RunID, cursor, func(_ uint64, event string, payload any) error {
			if event == dto.EventAgentRunEnded {
				ended++
				body, err := json.Marshal(payload)
				require.NoError(t, err)
				require.Contains(t, string(body), `"success":false`)
			}
			if event == dto.EventEnd {
				end++
			}
			return nil
		}))
		require.Equal(t, 1, ended)
		require.Equal(t, 1, end)
	}
}
