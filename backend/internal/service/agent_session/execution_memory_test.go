package agent_session

import (
	"context"
	"testing"
	"time"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

type memoryExecutor struct {
	call func(context.Context, Session, Message) (string, error)
}

func (e memoryExecutor) Execute(ctx context.Context, s Session, m Message) (string, error) {
	return e.call(ctx, s, m)
}

func TestExecutionMemoryPersistsAndResumesWithoutCallerState(t *testing.T) {
	svc, db, ctx, agent := fixture(t)
	tenant := reqctx.GetTenantUUID(ctx)
	require.NoError(t, db.Create(&capmodel.CapabilityRecord{CapabilityID: InvokeCapability, PluginID: "core", PluginVersion: "1", Title: "test", CapabilitiesHash: "invoke", ProtocolHash: "invoke", Status: "published"}).Error)
	require.NoError(t, db.Create(&capmodel.CapabilityRegistration{CapabilityID: InvokeCapability, TenantUUID: tenant, ContractRef: "test", Status: "published", Version: 1}).Error)
	require.NoError(t, db.Model(&setting.PluginInstanceConfig{}).Where("tenant_uuid = ? AND plugin_id = ?", tenant, "plugin.test").Update("value_json", datatypes.JSON([]byte(`{"client_id":"plugin.test","allowed_capabilities":["com.corex.agent.session.manage","com.corex.agent.invoke"]}`))).Error)
	session, err := svc.Create(ctx, agent, "")
	require.NoError(t, err)
	seen := make(chan *ExecutionMemory, 2)
	svc.executor = memoryExecutor{call: func(ctx context.Context, _ Session, _ Message) (string, error) {
		memory := ExecutionMemoryFromContext(ctx)
		seen <- memory
		if len(memory.History) == 0 {
			return "test-output", memory.SetPending(map[string]any{"status": "awaiting_params", "node_kind": "skill", "node_ref": "test.skill", "task_id": "task.test", "collected_params": map[string]any{"field": "test-value"}})
		}
		return "test-output", nil
	}}
	first, err := svc.Append(ctx, session.SessionUUID, "m1", "user", "test-input")
	require.NoError(t, err)
	run, err := svc.Invoke(ctx, session.SessionUUID, first.MessageUUID, "i1")
	require.NoError(t, err)
	waitSuccess := func(run Invocation) {
		require.Eventually(t, func() bool {
			out, e := svc.GetInvocation(ctx, session.SessionUUID, run.InvocationUUID)
			return e == nil && out.Status == "succeeded"
		}, 3*time.Second, 10*time.Millisecond)
	}
	waitSuccess(run)
	require.Empty(t, (<-seen).History)
	var saved m.ServiceSession
	require.NoError(t, db.First(&saved, "uuid = ?", session.SessionUUID).Error)
	require.Contains(t, string(saved.PendingTask), "task.test")
	require.NotNil(t, saved.PendingTaskExpiresAt)
	second, err := svc.Append(ctx, session.SessionUUID, "m2", "user", "test-next-input")
	require.NoError(t, err)
	run, err = svc.Invoke(ctx, session.SessionUUID, second.MessageUUID, "i2")
	require.NoError(t, err)
	waitSuccess(run)
	memory := <-seen
	require.Len(t, memory.History, 2)
	require.Equal(t, first.MessageUUID, memory.History[0].MessageUUID)
	require.Equal(t, "assistant", memory.History[1].Role)
	require.Equal(t, "task.test", memory.Pending["task_id"])
	saved = m.ServiceSession{}
	require.NoError(t, db.First(&saved, "uuid = ?", session.SessionUUID).Error)
	require.Equal(t, "null", string(saved.PendingTask))
	require.Nil(t, saved.PendingTaskExpiresAt)
	// Expired or malformed state is not silently dropped before a new execution.
	past := time.Now().Add(-time.Minute)
	require.NoError(t, db.Model(&saved).Updates(map[string]any{"pending_task": datatypes.JSON([]byte(`{"status":"awaiting_params"}`)), "pending_task_expires_at": past}).Error)
	_, err = svc.Invoke(ctx, session.SessionUUID, second.MessageUUID, "i3")
	require.ErrorIs(t, err, ErrContextExpired)
}
