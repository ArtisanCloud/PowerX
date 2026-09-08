package skills

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func evidenceDefinition() map[string]any {
	p := evidence.CalculationPolicy{Schema: evidence.PolicySchema, InputFields: []evidence.InputField{
		{Key: "a", Kind: "quantity", UnitTokens: []string{""}, LabelI18n: map[string]string{"en-US": "a"}, DescriptionI18n: map[string]string{"en-US": "a"}},
		{Key: "b", Kind: "quantity", UnitTokens: []string{""}, LabelI18n: map[string]string{"en-US": "b"}, DescriptionI18n: map[string]string{"en-US": "b"}},
	}, Formulas: []evidence.Formula{{Key: "rate", LabelI18n: map[string]string{"en-US": "rate"}, Expression: "n/d", Bindings: map[string]string{"n": "a", "d": "b"}, Precision: 2, Percent: true, WhenAnyPresent: []string{"a"}}}}
	return map[string]any{"schema": SkillDefinitionSchemaV2, "executor": map[string]any{"type": "llm_prompt", "output_mode": "response_envelope", "response_contract": evidence.ReportSchema, "evidence_sources": []string{"/message"}, "calculation_policy": p, "prompt_template_i18n": map[string]any{"en-US": "test"}}, "tool_dependencies": ToolRequirements{ToolDependencySchema, []ToolDependency{CalculatorDependency()}}}
}

func TestManifestEvidenceUsesToolAndIgnoresBusinessIdentity(t *testing.T) {
	for _, key := range []string{"customer.supply_chain", "user.education"} {
		ctx := evidence.WithLedger(context.Background())
		calls := 0
		executor := NewManifestExecutor(ManifestExecutorOptions{LLM: func(_ context.Context, in ManifestLLMInvocation) (string, error) {
			calls++
			if calls == 2 {
				require.Equal(t, evidence.NotesSchema, in.ResponseSchema["properties"].(map[string]any)["schema"].(map[string]any)["const"])
				require.NotContains(t, in.Payload, "source")
				require.Len(t, in.Payload["executed_calculations"], 1)
				b, err := json.Marshal(evidence.Notes{Schema: evidence.NotesSchema, Hypotheses: []string{}, Gaps: []string{}, Actions: []string{}})
				return string(b), err
			}
			require.Equal(t, evidence.ExtractionSchema, in.ResponseSchema["properties"].(map[string]any)["schema"].(map[string]any)["const"])
			b, err := json.Marshal(evidence.SourceSelection{Schema: evidence.ExtractionSchema, Data: map[string]evidence.SelectedValue{"a": {Scope: "s", TokenRef: "token_0"}, "b": {Scope: "s", TokenRef: "token_1"}}})
			return string(b), err
		}})
		out, err := executor.Execute(ctx, ExecuteInput{SkillID: key, TenantUUID: uuid.NewString(), Version: uuid.NewString(), TraceID: uuid.NewString(), Manifest: evidenceDefinition(), Context: map[string]any{"locale": "en-US"}, Payload: map[string]any{"message": "17 80"}})
		require.NoError(t, err)
		require.Equal(t, 2, calls)
		require.NoError(t, evidence.Verify(ctx, out["response_envelope"]))
		p := out["response_envelope"].(map[string]any)["presentation"].(map[string]any)
		require.Equal(t, "21.25%", p["computed"].([]any)[0].(map[string]any)["display_value"])
	}
}

func TestManifestEvidenceSelectionFailureCarriesTraceDetails(t *testing.T) {
	executor := NewManifestExecutor(ManifestExecutorOptions{LLM: func(_ context.Context, _ ManifestLLMInvocation) (string, error) {
		return `{"schema":"powerx.agent.evidence-source/v1","data":{"unknown":{"scope":"s","token_ref":"token_0"}}}`, nil
	}})
	_, err := executor.Execute(evidence.WithLedger(context.Background()), ExecuteInput{
		TenantUUID: uuid.NewString(), Version: uuid.NewString(), TraceID: uuid.NewString(), Manifest: evidenceDefinition(),
		Context: map[string]any{"locale": "en-US"}, Payload: map[string]any{"message": "17 80"},
	})
	var validation *EvidenceValidationError
	require.ErrorAs(t, err, &validation)
	require.ErrorContains(t, err, "evidence.source_input_key_unknown: unknown")
	require.Equal(t, "source_selection", validation.Details["stage"])
	selection, ok := validation.Details["selection"].(evidence.SourceSelection)
	require.True(t, ok)
	require.Contains(t, selection.Data, "unknown")
}

func TestDependenciesFailBeforeModelCall(t *testing.T) {
	called := false
	e := NewManifestExecutor(ManifestExecutorOptions{LLM: func(context.Context, ManifestLLMInvocation) (string, error) { called = true; return "", nil }})
	for _, change := range []func(map[string]any){func(m map[string]any) { delete(m, "tool_dependencies") }, func(m map[string]any) {
		m["executor"].(map[string]any)["response_contract"] = "powerx.agent.response/v3"
	}, func(m map[string]any) {
		d := CalculatorDependency()
		d.Version = "99"
		m["tool_dependencies"] = ToolRequirements{ToolDependencySchema, []ToolDependency{d}}
	}, func(m map[string]any) {
		d := CalculatorDependency()
		d.ToolKey = "missing.tool"
		m["tool_dependencies"] = ToolRequirements{ToolDependencySchema, []ToolDependency{d}}
	}} {
		m := evidenceDefinition()
		change(m)
		_, err := e.Execute(context.Background(), ExecuteInput{Manifest: m})
		require.Error(t, err)
	}
	require.False(t, called)
}

func TestExternalToolDependencyRequiresExplicitChecker(t *testing.T) {
	d := ToolDependency{ToolKey: "plugin.custom_algorithm", Version: "1.0.0", InputSchema: "input/v1", OutputSchema: "output/v1", Permission: "custom.compute"}
	m := map[string]any{"executor": map[string]any{"type": "capability"}, "tool_dependencies": ToolRequirements{ToolDependencySchema, []ToolDependency{d}}}
	require.Error(t, CheckToolDependencies(context.Background(), "tenant", m, nil))
	called := false
	err := CheckToolDependencies(context.Background(), "tenant", m, func(_ context.Context, tenant string, got ToolDependency) error {
		called = true
		require.Equal(t, "tenant", tenant)
		require.Equal(t, d, got)
		return nil
	})
	require.NoError(t, err)
	require.True(t, called)
}
