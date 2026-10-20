package skills

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	evidencesvc "github.com/ArtisanCloud/PowerX/internal/service/agent_evidence"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// 同一 tokenizer 产生的完整自然语言 token 必须能通过 typed 来源核验；与模型选择无关。
func TestV5NaturalLanguageTokensResolveWithoutModel(t *testing.T) {
	db := setupDefinitionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.EvidenceSnapshot{}))
	article, err := os.ReadFile("../../../tests/fixtures/agent_runtime/marketing_review_saas.txt")
	require.NoError(t, err)
	payload := map[string]any{"message": string(article)}
	tokens, err := evidence.TokenizeGenericSources(payload, []string{"/message"})
	require.NoError(t, err)
	var inventory evidence.GenericFactInventory
	require.NoError(t, json.Unmarshal([]byte(fixtureFactsInventory(evidence.GenericFactInventoryJSONSchema(tokens))), &inventory))
	facts, err := evidence.ResolveGenericFactInventory(inventory, tokens)
	require.NoError(t, err)
	definition := v5Definition(t)
	executor := NewManifestExecutor(ManifestExecutorOptions{Dependencies: NewRuntimeDependencyService(db), EvidenceStore: evidencesvc.NewService(db)})
	policy := definition["executor"].(map[string]any)["calculation_policy"].(evidence.CalculationPolicy)
	for _, datum := range facts {
		t.Run(datum.Source.Literal+"_"+datum.Unit, func(t *testing.T) {
			tenant := uuid.NewString()
			ctx := evidence.WithLedger(reqctx.WithUserUUID(reqctx.WithTenantUUID(context.Background(), tenant), uuid.NewString()))
			_, err := executor.prepareV5(ctx, ExecuteInput{TenantUUID: tenant, Version: uuid.NewString(), TraceID: uuid.NewString(), Manifest: definition, Payload: payload}, policy, evidence.Plan{Kind: "analysis", Data: []evidence.DraftDatum{datum}}, nil, "en-US")
			require.NoError(t, err, "literal=%s unit=%s quote=%s", datum.Source.Literal, datum.Unit, datum.Source.Quote)
		})
	}
}
