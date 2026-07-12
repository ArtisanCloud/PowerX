package skillsintegration

import (
	"context"
	"encoding/json"
	"testing"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestPluginSkillBridgeFailFastWritesAuditTrace(t *testing.T) {
	db := setupSkillsDB(t)
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
	invokeSvc := skillservice.NewInvokeService(registryRepo, auditSvc).AddExecutor(
		skillservice.NewPluginSkillHTTPExecutor(skillservice.StaticPluginEndpointResolver{}).
			WithDelegatedTokenProvider(func(ctx context.Context, provider string) (string, error) {
				return "sts-" + provider, nil
			}),
	)

	_, err = invokeSvc.Execute(context.Background(), skillservice.InvokeRequest{
		TenantUUID: "tenant-mediax",
		SkillID:    "mediax.video_rebuilder.cn",
		TraceID:    "trace-plugin-missing-context",
		InvokePath: "agent.plan.skill",
	}, map[string]interface{}{"urls": []interface{}{"https://example.com/a.mp4"}}, map[string]interface{}{
		"user_uuid": "user-001",
		"agent_id":  "1001",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), skillservice.ErrorCodePluginContextMissing)
	trace, err := traceRepo.GetByTraceID(context.Background(), "trace-plugin-missing-context")
	require.NoError(t, err)
	require.Equal(t, "failed", trace.Status)
	require.Equal(t, skillservice.ErrorCodePluginContextMissing, trace.ErrorCode)

	_, err = invokeSvc.Execute(context.Background(), skillservice.InvokeRequest{
		TenantUUID: "tenant-mediax",
		SkillID:    "mediax.video_rebuilder.cn",
		TraceID:    "trace-plugin-capability-mismatch",
		InvokePath: "agent.plan.skill",
	}, map[string]interface{}{}, map[string]interface{}{
		"user_uuid":     "user-001",
		"agent_id":      "1001",
		"session_id":    "session-001",
		"capability_id": "creation.other",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), skillservice.ErrorCodePluginCapabilityMismatch)

	_, err = invokeSvc.Execute(context.Background(), skillservice.InvokeRequest{
		TenantUUID: "tenant-mediax",
		SkillID:    "mediax.video_rebuilder.cn",
		TraceID:    "trace-plugin-not-installed",
		InvokePath: "agent.plan.skill",
	}, map[string]interface{}{}, map[string]interface{}{
		"user_uuid":     "user-001",
		"agent_id":      "1001",
		"session_id":    "session-001",
		"capability_id": "creation.video_automation.ingest",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), skillservice.ErrorCodePluginNotInstalled)
	trace, err = traceRepo.GetByTraceID(context.Background(), "trace-plugin-not-installed")
	require.NoError(t, err)
	require.Equal(t, skillservice.ErrorCodePluginNotInstalled, trace.ErrorCode)
}
