package runtime

import (
	"context"
	"testing"
	"time"

	bindingmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	sessions "github.com/ArtisanCloud/PowerX/internal/service/agent_session"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	agentmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fixedPlanningConfig struct{}

func (fixedPlanningConfig) ResolveForAgentChat(context.Context, string, *string, uint64, *dto.ChatConfig) (*dto.ChatConfig, error) {
	return &dto.ChatConfig{SystemPrompt: "agent instructions"}, nil
}

func TestDBPlanningInputLoaderUsesAdmittedRecordAndLiveGrant(t *testing.T) {
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for i := 0; i < 2; i++ {
		require.NoError(t, db.AutoMigrate(&agentmodel.ServiceSession{}, &agentmodel.ServiceMessage{}, &agentmodel.ServiceInvocation{},
			&capmodel.CapabilityRecord{}, &capmodel.CapabilityRegistration{}, &setting.PluginInstanceConfig{}, &bindingmodel.AgentSkillBinding{}))
	}
	require.NoError(t, db.Exec(`CREATE TABLE agents (id integer primary key, uuid text, tenant_uuid text, owner_plugin_id text, status text, env text, deleted_at datetime)`).Error)
	tenant, agentID := uuid.New(), uuid.New()
	require.NoError(t, db.Exec("INSERT INTO agents (id,uuid,tenant_uuid,owner_plugin_id,status,env) VALUES (?,?,?,?,?,?)",
		1, agentID, tenant, "plugin.test", "active", "dev").Error)
	session := agentmodel.ServiceSession{TenantUUID: tenant, PluginID: "plugin.test", ServiceActor: "client:plugin.test", AgentUUID: agentID, Status: "active"}
	require.NoError(t, db.Create(&session).Error)
	prior := agentmodel.ServiceMessage{TenantUUID: tenant, SessionUUID: session.UUID, AgentUUID: agentID, Role: "assistant", Content: "prior answer", Sequence: 1, IdempotencyKey: "previous", IdempotencyExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, db.Create(&prior).Error)
	message := agentmodel.ServiceMessage{TenantUUID: tenant, SessionUUID: session.UUID, AgentUUID: agentID, Role: "user", Content: "  new request  ", Sequence: 2, IdempotencyKey: "current", IdempotencyExpiresAt: time.Now().Add(time.Hour)}
	require.NoError(t, db.Create(&message).Error)
	anchor := agentmodel.ServiceInvocation{TenantUUID: tenant, SessionUUID: session.UUID, MessageUUID: message.UUID, AgentUUID: agentID,
		PluginID: "plugin.test", ServiceActor: "client:plugin.test", Status: "accepted", RunEnv: "dev", AdmissionState: "admitted",
		IdempotencyKey: "run", DeadlineAt: time.Now().Add(time.Hour), IdempotencyExpiresAt: time.Now().Add(time.Hour), TraceUUID: uuid.New()}
	require.NoError(t, db.Create(&anchor).Error)
	for _, capability := range []string{sessions.SessionCapability, sessions.InvokeCapability} {
		require.NoError(t, db.Create(&capmodel.CapabilityRecord{CapabilityID: capability, PluginID: "core", PluginVersion: "1", Title: "test", CapabilitiesHash: "hash", ProtocolHash: "hash", Status: "published"}).Error)
		require.NoError(t, db.Create(&capmodel.CapabilityRegistration{TenantUUID: tenant.String(), CapabilityID: capability, Status: "published", ContractRef: "test", Version: 1}).Error)
	}
	credential := setting.PluginInstanceConfig{TenantUUID: tenant.String(), PluginID: "plugin.test", Key: "auth.credentials", Enabled: true,
		ValueJSON: datatypes.JSON([]byte(`{"client_id":"plugin.test","allowed_capabilities":["com.corex.agent.session.manage","com.corex.agent.invoke"]}`))}
	require.NoError(t, db.Create(&credential).Error)
	loader := NewDBPlanningInputLoader(db)
	loader.config = fixedPlanningConfig{}
	ref := agent_run.TaskRef{TenantUUID: tenant.String(), Env: "dev", RunID: anchor.UUID.String(), TaskID: agent_run.PlanningTaskID, Attempt: 1}
	input, err := loader.Load(context.Background(), ref)
	require.NoError(t, err)
	require.Equal(t, message.Content, input.Message)
	require.Contains(t, input.Config.SystemPrompt, "prior answer")
	require.Contains(t, input.Config.SystemPrompt, "agent instructions")

	wrongEnv := ref
	wrongEnv.Env = "prod"
	_, err = loader.Load(context.Background(), wrongEnv)
	require.ErrorIs(t, err, sessions.ErrDependency)
	require.NoError(t, db.Model(&setting.PluginInstanceConfig{}).Where("id = ?", credential.ID).
		Update("value_json", datatypes.JSON([]byte(`{"client_id":"plugin.test","allowed_capabilities":["com.corex.agent.session.manage"]}`))).Error)
	_, err = loader.Load(context.Background(), ref)
	require.ErrorIs(t, err, sessions.ErrForbidden)
}
