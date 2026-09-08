package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	skillsvc "github.com/ArtisanCloud/PowerX/internal/service/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Explicit opt-in only: reads a published revision and runs the real staged pipeline.
// It is not a substitute for the complete team's HTTP/SSE/history acceptance.
func TestPublishedEvidenceLive(t *testing.T) {
	if os.Getenv("POWERX_EVIDENCE_LIVE") != "1" {
		t.Skip("POWERX_EVIDENCE_LIVE")
	}
	tenant, agentUUID, skillKey, input := os.Getenv("POWERX_TEST_TENANT_UUID"), os.Getenv("POWERX_TEST_AGENT_UUID"), os.Getenv("POWERX_TEST_SKILL_KEY"), os.Getenv("POWERX_TEST_EVIDENCE_INPUT")
	require.NotEmpty(t, input)
	require.NotEmpty(t, skillKey)
	providerDir, err := filepath.Abs("../../../../config/agents/providers.d")
	require.NoError(t, err)
	require.NoError(t, catalog.InitFromAppConfig(catalog.CatalogConfig{Dirs: []string{providerDir}, FailIfEmpty: true}, nil))
	db, err := gorm.Open(postgres.Open(os.Getenv("POWERX_TEST_DATABASE_DSN")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	var a agentmodel.Agent
	require.NoError(t, db.Where("tenant_uuid = ? AND uuid = ?", tenant, agentUUID).First(&a).Error)
	ctx, cancel := context.WithTimeout(context.Background(), 16*time.Minute)
	defer cancel()
	ctx = evidence.WithLedger(ctx)
	service := skillsvc.NewDefinitionInvokeService(skillrepo.NewSkillDefinitionRepository(db), newDefinitionManifestExecutor(db, "dev", nil, nil), nil)
	trace := uuid.NewString()
	out, err := service.Execute(ctx, skillsvc.InvokeRequest{TenantUUID: tenant, SkillID: skillKey, TraceID: trace}, map[string]any{"message": input}, map[string]any{"locale": "zh-CN", "agent_id": a.ID, "agent_uuid": agentUUID})
	var validation *skillsvc.EvidenceValidationError
	if errors.As(err, &validation) {
		b, _ := json.Marshal(validation.Draft)
		t.Logf("invalid_draft=%s", b)
	}
	require.NoError(t, err)
	require.NoError(t, evidence.Verify(ctx, out.Result["response_envelope"]))
	if expectedJSON := os.Getenv("POWERX_TEST_EXPECTED_COMPUTED"); expectedJSON != "" {
		var expected map[string]string
		require.NoError(t, json.Unmarshal([]byte(expectedJSON), &expected))
		body, err := json.Marshal(out.Result["response_envelope"])
		require.NoError(t, err)
		var report evidence.Report
		require.NoError(t, evidence.Decode(body, &report))
		actual := map[string]string{}
		for _, item := range report.Presentation.Computed {
			actual[item.Request.Key] = item.DisplayValue
		}
		require.Equal(t, expected, actual)
		if expectedConflict := os.Getenv("POWERX_TEST_EXPECTED_CONFLICT_KEY"); expectedConflict != "" {
			found := false
			for _, conflict := range report.Presentation.Conflicts {
				found = found || conflict.CalculationKey == expectedConflict
			}
			require.True(t, found)
		}
	}
	raw, err := json.Marshal(out.Result)
	require.NoError(t, err)
	t.Logf("trace_id=%s result=%s", trace, raw)
}
