package skills

import (
	"context"
	"encoding/json"
	evidencesvc "github.com/ArtisanCloud/PowerX/internal/service/agent_evidence"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	agentconfig "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	modelconfig "github.com/ArtisanCloud/PowerX/internal/server/ai/drivers/config"
	"github.com/ArtisanCloud/PowerX/internal/server/ai/factory/llm"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// 可选真实模型检查，共享开发环境的物理容量池；不写业务记录、不重放历史 Run。
func TestRealOllamaEvidenceSaaSReview(t *testing.T) {
	path := os.Getenv("POWERX_TEST_EVIDENCE_RUNTIME_CONFIG")
	if path == "" {
		t.Skip("set private runtime config path for real model validation")
	}
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
	require.NotEmpty(t, cfg.AI.Runtime.PhysicalModelPools)
	require.NotEmpty(t, cfg.Queue.Redis.Addr)
	client := redis.NewClient(&redis.Options{Addr: cfg.Queue.Redis.Addr, Password: cfg.Queue.Redis.Password, DB: cfg.Queue.Redis.DB})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	ctx = reqctx.WithUserUUID(reqctx.WithEnv(reqctx.WithTenantUUID(ctx, uuid.NewString()), "dev"), uuid.NewString())
	db := setupDefinitionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.EvidenceSnapshot{}))
	checks := NewRuntimeDependencyService(db)
	store := evidencesvc.NewService(db)
	repository := skillrepo.NewSkillDefinitionRepository(db)
	definitions := NewDefinitionService(repository, checks)
	t.Logf("real model provider=%s model=%s", cfg.AI.Runtime.PhysicalModelPools[0].Provider, cfg.AI.Runtime.PhysicalModelPools[0].Model)
	require.NoError(t, llm.ConfigurePhysicalModelPools(ctx, client, cfg.AI.Runtime.PhysicalModelPools, time.Minute))
	rule := cfg.AI.Runtime.PhysicalModelPools[0]
	require.Equal(t, "ollama", rule.Provider)
	article, err := os.ReadFile("../../../tests/fixtures/agent_runtime/marketing_review_saas.txt")
	require.NoError(t, err)
	raw, err = os.ReadFile("../../../cmd/database/seed/locales/marketing_calculation_policy.json")
	require.NoError(t, err)
	var policy map[string]any
	require.NoError(t, json.Unmarshal(raw, &policy))
	manifest := v5Definition(t)
	executorConfig := manifest["executor"].(map[string]any)
	executorConfig["calculation_policy"] = policy
	executorConfig["prompt_template_i18n"] = map[string]any{"zh-CN": "test"}
	calls := 0
	executor := NewManifestExecutor(ManifestExecutorOptions{Dependencies: checks, EvidenceStore: store, LLM: func(ctx context.Context, in ManifestLLMInvocation) (string, error) {
		calls++
		t.Logf("real evidence stage %d started", calls)
		payload, err := json.Marshal(in.Payload)
		if err != nil {
			return "", err
		}
		result, err := llm.Invoke(ctx, &modelconfig.ModelConfig{Provider: rule.Provider, Endpoint: rule.Endpoint, Model: rule.Model, SystemPrompt: in.PromptTemplate, Temperature: 0, TemperatureSet: true, MaxTokens: 4096, ResponseSchema: in.ResponseSchema, Timeout: 5 * time.Minute, MaxConcurrentRequests: 1, Extra: map[string]any{"think": false}}, string(payload))
		if err != nil {
			return "", err
		}
		t.Logf("real evidence stage %d completed", calls)
		return result.Text, nil
	}})

	publisher := NewPackagePublisher(&memorySkillPackageStore{})
	source := createAgentAuthoringSource(t, definitions, reqctx.GetTenantUUID(ctx), reqctx.GetUserUUID(ctx))
	draft, revision, err := definitions.CreateDraft(ctx, CreateDefinitionDraftInput{TenantUUID: reqctx.GetTenantUUID(ctx), SkillID: "customer.actual_review", DisplayNameI18n: map[string]string{"zh-CN": "fixture"}, DescriptionI18n: map[string]string{"zh-CN": "fixture"}, SourceKind: skillmodel.SkillPackageSourceAgentAuthoring, PackageSourceUUID: source.UUID.String(), Definition: manifest, AuthorMemberUUID: reqctx.GetUserUUID(ctx)})
	require.NoError(t, err)
	published, err := publisher.PublishCanonical(ctx, CanonicalSkillPackageInput{TenantUUID: reqctx.GetTenantUUID(ctx), SkillID: draft.SkillID, RevisionUUID: revision.UUID.String(), DisplayName: "fixture", Description: "fixture", Definition: manifest})
	require.NoError(t, err)
	_, _, err = definitions.PublishCurrentRevision(ctx, PublishDefinitionInput{TenantUUID: reqctx.GetTenantUUID(ctx), DraftUUID: draft.UUID.String(), ArtifactURI: published.ArtifactURI, Checksum: published.Checksum, UpdatedByMemberUUID: reqctx.GetUserUUID(ctx)})
	require.NoError(t, err)
	invoker := NewDefinitionInvokeService(repository, executor, nil)
	for _, sample := range []struct {
		name, text string
		want       []string
	}{
		{"original", string(article), []string{"1.35", "0.85"}},
		{"changed", strings.NewReplacer("34.2", "20.0", "46.2", "60.0", "29.0", "10.0").Replace(string(article)), []string{"3.00", "0.50"}},
	} {
		t.Run(sample.name, func(t *testing.T) {
			runCtx := evidence.WithInvocationScope(evidence.WithLedger(ctx), uuid.NewString(), "review")
			before := calls
			out, err := invoker.Execute(runCtx, InvokeRequest{TenantUUID: reqctx.GetTenantUUID(ctx), SkillID: draft.SkillID, Version: revision.UUID.String(), TraceID: uuid.NewString()}, map[string]any{"message": sample.text}, map[string]any{"locale": "zh-CN"})
			require.NoError(t, err)
			require.Equal(t, 3, calls-before)
			require.NoError(t, evidence.Verify(runCtx, out.Result["response_envelope"]))
			raw, err := json.Marshal(out.Result["response_envelope"])
			require.NoError(t, err)
			var report evidence.ReportV5
			require.NoError(t, evidence.DecodeToolJSON(raw, &report))
			values := []string{}
			for _, datum := range report.Presentation.Computed {
				values = append(values, datum.Value)
			}
			// 每个结果来自实际 typed calculator；不根据模型自由文本检查或补算。
			for _, want := range sample.want {
				found := false
				for _, value := range values {
					found = found || numericTestEqual(value, want)
				}
				require.True(t, found, "missing expected result %s in %v", want, values)
			}
			require.Len(t, report.Receipts, 2)
			require.NotEmpty(t, report.Presentation.Conflicts)
			ref := out.Result["response_evidence"].(evidence.EvidenceReference)
			archived, err := evidencesvc.NewService(db).Read(ctx, ref)
			require.NoError(t, err)
			require.Equal(t, report, *archived.Report)
			require.Equal(t, 3, calls-before)
			t.Logf("tool_contract_fixture %s", mustV5JSON(t, map[string]any{"definition": "report", "value": report}))
		})
	}
}
func numericTestEqual(a, b string) bool {
	x, xok := new(big.Rat).SetString(a)
	y, yok := new(big.Rat).SetString(b)
	return xok && yok && x.Cmp(y) == 0
}
