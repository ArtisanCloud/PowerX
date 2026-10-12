package skills

import (
	"context"
	"encoding/json"
	"os"
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
	ctx = evidence.WithLedger(reqctx.WithEnv(reqctx.WithTenantUUID(ctx, uuid.NewString()), "dev"))
	require.NoError(t, llm.ConfigurePhysicalModelPools(ctx, client, cfg.AI.Runtime.PhysicalModelPools, time.Minute))
	rule := cfg.AI.Runtime.PhysicalModelPools[0]
	require.Equal(t, "ollama", rule.Provider)
	article, err := os.ReadFile("../../../tests/fixtures/agent_runtime/marketing_review_saas.txt")
	require.NoError(t, err)
	raw, err = os.ReadFile("../../../cmd/database/seed/locales/marketing_calculation_policy.json")
	require.NoError(t, err)
	var policy map[string]any
	require.NoError(t, json.Unmarshal(raw, &policy))
	manifest := evidenceDefinition()
	executorConfig := manifest["executor"].(map[string]any)
	executorConfig["calculation_policy"] = policy
	executorConfig["prompt_template_i18n"] = map[string]any{"zh-CN": "test"}
	calls := 0
	executor := NewManifestExecutor(ManifestExecutorOptions{LLM: func(ctx context.Context, in ManifestLLMInvocation) (string, error) {
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
	out, err := executor.Execute(ctx, ExecuteInput{SkillID: "marketing.review_summarize", TenantUUID: reqctx.GetTenantUUID(ctx), Version: uuid.NewString(), TraceID: uuid.NewString(), Manifest: manifest, Context: map[string]any{"locale": "zh-CN"}, Payload: map[string]any{"message": string(article)}})
	require.NoError(t, err)
	require.NoError(t, evidence.Verify(ctx, out["response_envelope"]))
	presentation := out["response_envelope"].(map[string]any)["presentation"].(map[string]any)
	values := []string{}
	for _, r := range presentation["reported"].([]any) {
		values = append(values, r.(map[string]any)["value"].(string))
	}
	for _, v := range []string{"6000", "2.4", "34.2", "46.2", "1.35", "3.37", "29.0", "27.8", "0.65", "4.8", "1.2", "0.08"} {
		require.Contains(t, values, v)
	}
	require.Len(t, presentation["computed"], 2)
	require.Len(t, presentation["conflicts"], 1)
	t.Logf("real evidence passed: reported=%d computed=%d conflicts=%d gaps=%v", len(values), len(presentation["computed"].([]any)), len(presentation["conflicts"].([]any)), presentation["gaps"])
}
