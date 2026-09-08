package agent_session

import (
	"context"
	"testing"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestExpiredOrphanExecutionDoesNotBlockSession(t *testing.T) {
	svc, db, ctx, agent := fixture(t)
	session, err := svc.Create(ctx, agent, "")
	require.NoError(t, err)
	message, err := svc.Append(ctx, session.SessionUUID, "m", "user", "test")
	require.NoError(t, err)
	run := &m.ServiceInvocation{TenantUUID: uuid.MustParse(reqctx.GetTenantUUID(ctx)), SessionUUID: session.SessionUUID, AgentUUID: agent, MessageUUID: message.MessageUUID, PluginID: "plugin.test", ServiceActor: "client:plugin.test", Status: "running", IdempotencyKey: "orphan", RequestHash: message.MessageUUID.String(), DeadlineAt: time.Now().Add(-time.Minute), IdempotencyExpiresAt: time.Now().Add(time.Hour), TraceUUID: uuid.New()}
	require.NoError(t, db.Create(run).Error)
	_, err = svc.Mutate(ctx, session.SessionUUID, "archive", "")
	require.NoError(t, err)
	require.NoError(t, db.First(run, "uuid = ?", run.UUID).Error)
	require.Equal(t, "failed", run.Status)
	require.Equal(t, "AGENT_SESSION_EXECUTION_EXPIRED", run.ReasonCode)
	require.NotNil(t, run.FinishedAt)
}

type controlledExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (e *controlledExecutor) Execute(ctx context.Context, _ Session, _ Message) (string, error) {
	close(e.started)
	select {
	case <-e.release:
		return "test-output", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
func TestInvocationIdempotencyDisconnectAndCancel(t *testing.T) {
	for _, cancelExecution := range []bool{false, true} {
		t.Run(map[bool]string{false: "disconnect_does_not_cancel", true: "explicit_cancel"}[cancelExecution], func(t *testing.T) {
			svc, db, ctx, agent := fixture(t)
			require.NoError(t, db.Create(&capmodel.CapabilityRecord{CapabilityID: InvokeCapability, PluginID: "core", PluginVersion: "1", Title: "test", CapabilitiesHash: "hash", ProtocolHash: "hash", Status: "published"}).Error)
			require.NoError(t, db.Create(&capmodel.CapabilityRegistration{TenantUUID: reqctx.GetTenantUUID(ctx), CapabilityID: InvokeCapability, Status: "published", ContractRef: "test", Version: 1}).Error)
			require.NoError(t, db.Model(&setting.PluginInstanceConfig{}).Where("plugin_id = ?", "plugin.test").Update("value_json", datatypes.JSON([]byte(`{"client_id":"plugin.test","allowed_capabilities":["com.corex.agent.session.manage","com.corex.agent.invoke"]}`))).Error)
			executor := &controlledExecutor{started: make(chan struct{}), release: make(chan struct{})}
			svc.executor = executor
			session, err := svc.Create(ctx, agent, "")
			require.NoError(t, err)
			message, err := svc.Append(ctx, session.SessionUUID, "message", "user", "test")
			require.NoError(t, err)
			requestCtx, disconnect := context.WithCancel(ctx)
			run, err := svc.Invoke(requestCtx, session.SessionUUID, message.MessageUUID, "run")
			require.NoError(t, err)
			select {
			case <-executor.started:
			case <-time.After(2 * time.Second):
				t.Fatal("executor_start_timeout")
			}
			disconnect()
			repeat, err := svc.Invoke(ctx, session.SessionUUID, message.MessageUUID, "run")
			require.NoError(t, err)
			require.Equal(t, run.InvocationUUID, repeat.InvocationUUID)
			_, err = svc.Invoke(ctx, session.SessionUUID, message.MessageUUID, "different-key")
			require.ErrorIs(t, err, ErrConflict)
			_, err = svc.Mutate(ctx, session.SessionUUID, "delete", "")
			require.ErrorIs(t, err, ErrConflict)
			if cancelExecution {
				_, err = svc.Cancel(ctx, session.SessionUUID, run.InvocationUUID)
				require.NoError(t, err)
			} else {
				close(executor.release)
			}
			want := "succeeded"
			if cancelExecution {
				want = "cancelled"
			}
			require.Eventually(t, func() bool {
				state, err := svc.GetInvocation(ctx, session.SessionUUID, run.InvocationUUID)
				return err == nil && state.Status == want
			}, 3*time.Second, 20*time.Millisecond)
			messages, _, err := svc.Messages(ctx, session.SessionUUID, 1, 10)
			require.NoError(t, err)
			if cancelExecution {
				require.Len(t, messages, 1)
			} else {
				require.Len(t, messages, 2)
				require.Equal(t, "assistant", messages[1].Role)
				require.Equal(t, "test-output", messages[1].Content)
			}
		})
	}
}
