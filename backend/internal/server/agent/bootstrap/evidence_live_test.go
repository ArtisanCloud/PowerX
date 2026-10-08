package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	agentconfig "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/factory/llm"
	skillsvc "github.com/ArtisanCloud/PowerX/internal/service/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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
	ctx = reqctx.WithEnv(reqctx.WithTenantUUID(ctx, tenant), "dev")
	if path := os.Getenv("POWERX_TEST_EVIDENCE_RUNTIME_CONFIG"); path != "" {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		var cfg struct {
			AI    agentconfig.AIConfig `yaml:"ai"`
			Queue struct {
				Redis struct {
					Addr     string `yaml:"addr"`
					Password string `yaml:"password"`
					DB       int    `yaml:"db"`
				} `yaml:"redis"`
			} `yaml:"queue"`
		}
		require.NoError(t, yaml.Unmarshal(raw, &cfg))
		client := redis.NewClient(&redis.Options{Addr: cfg.Queue.Redis.Addr, Password: cfg.Queue.Redis.Password, DB: cfg.Queue.Redis.DB})
		t.Cleanup(func() { _ = client.Close() })
		require.NotEmpty(t, cfg.AI.Runtime.PhysicalModelPools)
		require.NoError(t, llm.ConfigurePhysicalModelPools(ctx, client, cfg.AI.Runtime.PhysicalModelPools, time.Minute))
	}
	service := skillsvc.NewDefinitionInvokeService(skillrepo.NewSkillDefinitionRepository(db), newDefinitionManifestExecutor(db, "dev", nil, nil), nil)
	trace := uuid.NewString()
	payload := map[string]any{"message": input}
	if path := os.Getenv("POWERX_TEST_EVIDENCE_UPSTREAM_FILE"); path != "" {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		var upstream map[string]any
		require.NoError(t, json.Unmarshal(raw, &upstream))
		for key, value := range upstream {
			require.True(t, strings.HasPrefix(key, "upstream_"))
			payload[key] = value
		}
	}
	out, err := service.Execute(ctx, skillsvc.InvokeRequest{TenantUUID: tenant, SkillID: skillKey, TraceID: trace}, payload, map[string]any{"locale": "zh-CN", "agent_id": a.ID, "agent_uuid": agentUUID})
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
	if path := os.Getenv("POWERX_TEST_EVIDENCE_RESULT_FILE"); path != "" {
		require.NoError(t, os.WriteFile(path, raw, 0600))
	}
	if os.Getenv("POWERX_TEST_EVIDENCE_FIX_REVIEW") == "1" {
		body, err := json.Marshal(out.Result["response_envelope"])
		require.NoError(t, err)
		var report evidence.Report
		require.NoError(t, evidence.Decode(body, &report))
		fields := map[string]evidence.Datum{}
		for _, item := range report.Presentation.Reported {
			fields[item.Key] = item
		}
		require.Len(t, fields, 15)
		require.Equal(t, "客单价下限", fields["unit_price_lower"].Label)
		require.Equal(t, "6000", fields["unit_price_lower"].Value)
		require.Equal(t, "2.4", fields["unit_price_upper"].Value)
		require.Equal(t, "未续费或未升级回溯期", fields["inactivity_window"].Label)
		require.Equal(t, "二次购买观察期", fields["repeat_window"].Label)
		require.Equal(t, "短信渠道", fields["reported_sms_roi"].Scope)
		require.Equal(t, "信息流渠道", fields["reported_feed_roi"].Scope)
		hypotheses := strings.Join(report.Presentation.Hypotheses, "\n")
		require.Regexp(t, "老客|存量", hypotheses)
		require.Regexp(t, "信息流|围观", hypotheses)
		require.NotContains(t, hypotheses, "〔原文数值〕")
		require.NotContains(t, hypotheses, "点击率低")
		require.NotContains(t, hypotheses, "复购率提升可能")
		require.Len(t, report.Presentation.Hypotheses, 4)
		require.Len(t, report.Presentation.Gaps, 1)
		actions := strings.Join(report.Presentation.Actions, "\n")
		require.NotContains(t, actions, "核对活动产投比的计算")
		require.NotContains(t, actions, "验证原文财务ROI的计算")
		require.Len(t, report.Presentation.Actions, 4)
		require.Len(t, report.Presentation.Computed, 2)
		require.Len(t, report.Presentation.Conflicts, 1)
	}
	t.Logf("trace_id=%s result=%s", trace, raw)
}
