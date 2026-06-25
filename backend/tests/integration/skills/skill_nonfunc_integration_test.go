package skillsintegration

import (
	"context"
	"fmt"
	"testing"
	"time"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/stretchr/testify/require"
)

func TestSkillNonFuncBaseline_ImportInvokeAudit(t *testing.T) {
	db := setupSkillsDB(t)
	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(db)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(db)
	auditSvc := skillservice.NewAuditTraceService(traceRepo, auditRepo)
	importSvc := skillservice.NewImportService(registryRepo, auditSvc)
	lifecycleSvc := skillservice.NewLifecycleService(registryRepo, auditSvc)
	invokeSvc := skillservice.NewInvokeService(registryRepo, auditSvc)

	ctx := context.Background()
	const (
		totalImports   = 80
		totalPublishes = 30
	)
	var auditBefore int64
	require.NoError(t, db.Model(&skillmodel.SkillLifecycleAudit{}).Count(&auditBefore).Error)
	var traceBefore int64
	require.NoError(t, db.Model(&skillmodel.SkillExecutionTrace{}).Where("status = ?", "completed").Count(&traceBefore).Error)

	importStart := time.Now()
	for i := 0; i < totalImports; i++ {
		skillID := fmt.Sprintf("skill.nonfunc.%03d.incident-triage", i)
		_, err := importSvc.ImportDraft(ctx, skillservice.ImportRequest{
			SkillID:    skillID,
			Version:    "1.0.0",
			Source:     skillmodel.SkillSourcePlugin,
			BundleURI:  fmt.Sprintf("s3://skills/%s-1.0.0.tgz", skillID),
			Checksum:   fmt.Sprintf("sha256:nonfunc-%03d", i),
			Operator:   "nonfunc-importer",
			ImportType: skillservice.ImportTypeUpload,
		})
		require.NoError(t, err)
	}
	importElapsed := time.Since(importStart)
	require.Less(t, importElapsed, 8*time.Second, "import baseline exceeded")

	publishStart := time.Now()
	for i := 0; i < totalPublishes; i++ {
		skillID := fmt.Sprintf("skill.nonfunc.%03d.incident-triage", i)
		err := lifecycleSvc.Publish(ctx, skillID, "1.0.0", "nonfunc-publisher", "baseline publish")
		require.NoError(t, err)
	}
	publishElapsed := time.Since(publishStart)
	require.Less(t, publishElapsed, 8*time.Second, "publish baseline exceeded")

	invokeStart := time.Now()
	success := 0
	for i := 0; i < totalPublishes; i++ {
		skillID := fmt.Sprintf("skill.nonfunc.%03d.incident-triage", i)
		out, err := invokeSvc.Execute(ctx, skillservice.InvokeRequest{
			TenantUUID: "tenant-nonfunc",
			SkillID:    skillID,
			InvokePath: "tenant.skills.invoke",
		}, map[string]interface{}{"incident_id": fmt.Sprintf("INC-%04d", i), "context": "baseline warning"}, nil)
		require.NoError(t, err)
		require.Equal(t, "completed", out.Status)
		success++
	}
	invokeElapsed := time.Since(invokeStart)
	require.Less(t, invokeElapsed, 8*time.Second, "invoke baseline exceeded")
	require.Equal(t, totalPublishes, success)

	var auditCount int64
	require.NoError(t, db.Model(&skillmodel.SkillLifecycleAudit{}).Count(&auditCount).Error)
	require.Equal(t, int64(totalImports+totalPublishes), auditCount-auditBefore, "lifecycle audit write count mismatch")

	var traceCount int64
	require.NoError(t, db.Model(&skillmodel.SkillExecutionTrace{}).Where("status = ?", "completed").Count(&traceCount).Error)
	require.Equal(t, int64(totalPublishes), traceCount-traceBefore, "resolved trace count mismatch")
}
