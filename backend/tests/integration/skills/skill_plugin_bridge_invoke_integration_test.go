package skillsintegration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestPluginSkillBridgeInvokeCallsExecutorWithFullContext(t *testing.T) {
	db := setupSkillsDB(t)
	var captured map[string]interface{}
	pluginServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/plugin/skills/invoke", r.URL.Path)
		require.Equal(t, "Bearer sts-com.powerx.plugin.mediax-studio", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&captured))
		ctxMap := captured["context"].(map[string]interface{})
		for _, field := range []string{"tenant_uuid", "user_uuid", "agent_id", "session_id", "skill_id", "trace_id"} {
			require.NotEmpty(t, ctxMap[field], field)
		}
		require.Equal(t, "creation.video_automation.ingest", ctxMap["capability"])
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  true,
			"skill_id": "mediax.video_rebuilder.cn",
			"task_id":  "video-automation-task-001",
			"status":   "queued",
			"message":  "已创建视频重构任务",
			"data":     map[string]interface{}{"output_path": "s3://mediax/out.mp4"},
		})
	}))
	defer pluginServer.Close()

	manifestRaw, err := json.Marshal(pluginManifestFixture("mediax.video_rebuilder.cn", "com.powerx.plugin.mediax-studio"))
	require.NoError(t, err)
	require.NoError(t, db.Create(&skillmodel.SkillRegistryRecord{
		SkillID:           "mediax.video_rebuilder.cn",
		Version:           "1.0.0",
		Source:            skillmodel.SkillSourcePlugin,
		Status:            skillmodel.SkillStatusPublished,
		IsLatestPublished: true,
		BundleURI:         "plugin://com.powerx.plugin.mediax-studio/mediax.video_rebuilder.cn/1.0.0",
		Checksum:          "sha256:mediax-plugin-skill",
		ManifestJSON:      datatypes.JSON(manifestRaw),
		ImportType:        "upload",
		UpdatedBy:         "seed",
	}).Error)

	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(db)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(db)
	auditSvc := skillservice.NewAuditTraceService(traceRepo, auditRepo)
	pluginExecutor := skillservice.NewPluginSkillHTTPExecutor(skillservice.StaticPluginEndpointResolver{
		"com.powerx.plugin.mediax-studio": pluginServer.URL,
	}).WithDelegatedTokenProvider(func(ctx context.Context, provider string) (string, error) {
		return "sts-" + provider, nil
	})
	invokeSvc := skillservice.NewInvokeService(registryRepo, auditSvc).AddExecutor(pluginExecutor)

	out, err := invokeSvc.Execute(context.Background(), skillservice.InvokeRequest{
		TenantUUID: "tenant-mediax",
		SkillID:    "mediax.video_rebuilder.cn",
		TraceID:    "trace-mediax-plugin-001",
		InvokePath: "agent.plan.skill",
	}, map[string]interface{}{
		"urls": []interface{}{"https://example.com/a.mp4"},
	}, map[string]interface{}{
		"user_uuid":     "user-001",
		"agent_id":      "1001",
		"session_id":    "session-001",
		"message_id":    "message-001",
		"capability_id": "creation.video_automation.ingest",
	})
	require.NoError(t, err)
	require.Equal(t, "completed", out.Status)
	require.Equal(t, "queued", out.Result["status"])
	require.Equal(t, "video-automation-task-001", out.Result["task_id"])
	require.Equal(t, "mediax.video_rebuilder.cn", captured["skill_id"])

	trace, err := traceRepo.GetByTraceID(context.Background(), "trace-mediax-plugin-001")
	require.NoError(t, err)
	require.Equal(t, "com.powerx.plugin.mediax-studio", trace.ProviderPluginID)
	require.Equal(t, "1001", trace.AgentID)
	require.Equal(t, "session-001", trace.SessionID)
	require.Equal(t, "message-001", trace.MessageID)
	require.Equal(t, "/api/v1/creation/video-automation/ingest", trace.ExecutorPath)
	require.Equal(t, "video-automation-task-001", trace.PluginTaskID)
}
