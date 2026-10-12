package skills

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func evidenceDefinition() map[string]any {
	field := func(key string) evidence.InputField {
		return evidence.InputField{Key: key, Kind: "quantity", UnitTokens: []string{""}, LabelI18n: map[string]string{"en-US": key}, DescriptionI18n: map[string]string{"en-US": key}, EvidenceTermsI18n: map[string][]string{"en-US": {key}}, AppliesTo: []string{"test_profile"}}
	}
	p := evidence.CalculationPolicy{Schema: evidence.PolicySchema, ActivityProfiles: []evidence.ActivityProfile{{Key: "test_profile", LabelI18n: map[string]string{"en-US": "test"}, EvidenceAnyI18n: map[string][]string{"en-US": {"test"}}}}, InputFields: []evidence.InputField{field("a"), field("b")}, Formulas: []evidence.Formula{{Key: "rate", LabelI18n: map[string]string{"en-US": "rate"}, Expression: "n/d", Bindings: map[string]string{"n": "a", "d": "b"}, Precision: 2, Percent: true, WhenAnyPresent: []string{"a"}, AppliesTo: []string{"test_profile"}}}}
	return map[string]any{"schema": SkillDefinitionSchemaV2, "executor": map[string]any{"type": "llm_prompt", "output_mode": "response_envelope", "response_contract": evidence.ReportSchema, "evidence_sources": []string{"/message"}, "calculation_policy": p, "prompt_template_i18n": map[string]any{"en-US": "test"}}, "tool_dependencies": ToolRequirements{ToolDependencySchema, []ToolDependency{CalculatorDependency()}}}
}

func TestManifestEvidenceUsesToolAndIgnoresBusinessIdentity(t *testing.T) {
	for _, key := range []string{"customer.supply_chain", "user.education"} {
		ctx := evidence.WithLedger(context.Background())
		calls := 0
		executor := NewManifestExecutor(ManifestExecutorOptions{LLM: func(_ context.Context, in ManifestLLMInvocation) (string, error) {
			calls++
			if calls == 3 {
				require.Equal(t, evidence.NotesSchema, in.ResponseSchema["properties"].(map[string]any)["schema"].(map[string]any)["const"])
				require.NotContains(t, in.Payload, "source")
				require.Len(t, in.Payload["executed_calculations"], 1)
				b, err := json.Marshal(evidence.Notes{Schema: evidence.NotesSchema, Hypotheses: []string{}, Gaps: []string{}, Actions: []string{}})
				return string(b), err
			}
			if calls == 1 {
				require.Equal(t, evidence.GenericFactInventorySchema, in.ResponseSchema["properties"].(map[string]any)["schema"].(map[string]any)["const"])
				return fixtureFactsInventory(in.ResponseSchema), nil
			}
			require.Equal(t, evidence.ExtractionSchema, in.ResponseSchema["properties"].(map[string]any)["schema"].(map[string]any)["const"])
			b, err := json.Marshal(evidence.SourceSelection{Schema: evidence.ExtractionSchema, Data: map[string]evidence.SelectedValue{"a": {Scope: "s", TokenRef: "token_0"}, "b": {Scope: "s", TokenRef: "token_1"}}})
			return string(b), err
		}})
		out, err := executor.Execute(ctx, ExecuteInput{SkillID: key, TenantUUID: uuid.NewString(), Version: uuid.NewString(), TraceID: uuid.NewString(), Manifest: evidenceDefinition(), Context: map[string]any{"locale": "en-US"}, Payload: map[string]any{"message": "test a:17; b:80"}})
		require.NoError(t, err)
		require.Equal(t, 3, calls)
		require.NoError(t, evidence.Verify(ctx, out["response_envelope"]))
		p := out["response_envelope"].(map[string]any)["presentation"].(map[string]any)
		require.Equal(t, "21.25%", p["computed"].([]any)[0].(map[string]any)["display_value"])
	}
}

func TestManifestEvidencePreservesUnlistedNumericFactWithoutCalculatingIt(t *testing.T) {
	ctx := evidence.WithLedger(context.Background())
	calls := 0
	executor := NewManifestExecutor(ManifestExecutorOptions{LLM: func(_ context.Context, in ManifestLLMInvocation) (string, error) {
		calls++
		switch calls {
		case 1:
			return fixtureFactsInventory(in.ResponseSchema), nil
		case 2:
			b, err := json.Marshal(evidence.SourceSelection{Schema: evidence.ExtractionSchema, Data: map[string]evidence.SelectedValue{"a": {Scope: "campaign", TokenRef: "token_0"}, "b": {Scope: "campaign", TokenRef: "token_1"}}})
			return string(b), err
		default:
			b, err := json.Marshal(evidence.Notes{Schema: evidence.NotesSchema, Hypotheses: []string{}, Gaps: []string{}, Actions: []string{}})
			return string(b), err
		}
	}})
	out, err := executor.Execute(ctx, ExecuteInput{SkillID: "customer.any", TenantUUID: uuid.NewString(), Version: uuid.NewString(), TraceID: uuid.NewString(), Manifest: evidenceDefinition(), Context: map[string]any{"locale": "en-US"}, Payload: map[string]any{"message": "test a:17; b:80; dwell time 45 seconds"}})
	require.NoError(t, err)
	presentation := out["response_envelope"].(map[string]any)["presentation"].(map[string]any)
	require.Len(t, presentation["reported"], 3)
	require.Equal(t, "dwell time", presentation["reported"].([]any)[2].(map[string]any)["label"])
	require.Equal(t, "45", presentation["reported"].([]any)[2].(map[string]any)["value"])
	require.Len(t, presentation["computed"], 1)
}

func TestManifestEvidencePreservesGenericFactWhenNoCalculationProfileMatches(t *testing.T) {
	ctx := evidence.WithLedger(context.Background())
	calls := 0
	executor := NewManifestExecutor(ManifestExecutorOptions{LLM: func(_ context.Context, in ManifestLLMInvocation) (string, error) {
		calls++
		switch calls {
		case 1:
			return fixtureFactsInventory(in.ResponseSchema), nil
		case 2:
			properties := in.ResponseSchema["properties"].(map[string]any)["data"].(map[string]any)["properties"].(map[string]any)
			require.Empty(t, properties)
			return `{"schema":"powerx.agent.evidence-source/v1","data":{}}`, nil
		default:
			return `{"schema":"powerx.agent.evidence-notes/v1","hypotheses":[],"gaps":[],"actions":[]}`, nil
		}
	}})
	out, err := executor.Execute(ctx, ExecuteInput{SkillID: "customer.any", TenantUUID: uuid.NewString(), Version: uuid.NewString(), TraceID: uuid.NewString(), Manifest: evidenceDefinition(), Context: map[string]any{"locale": "en-US"}, Payload: map[string]any{"message": "dwell time 45 seconds"}})
	require.NoError(t, err)
	presentation := out["response_envelope"].(map[string]any)["presentation"].(map[string]any)
	require.Len(t, presentation["reported"], 1)
	require.Empty(t, presentation["computed"])
}

func TestManifestEvidenceSelectionFailureCarriesTraceDetails(t *testing.T) {
	calls := 0
	executor := NewManifestExecutor(ManifestExecutorOptions{LLM: func(_ context.Context, in ManifestLLMInvocation) (string, error) {
		calls++
		if calls == 1 {
			return fixtureFactsInventory(in.ResponseSchema), nil
		}
		return `{"schema":"powerx.agent.evidence-source/v1","data":{"unknown":{"scope":"s","token_ref":"token_0"}}}`, nil
	}})
	_, err := executor.Execute(evidence.WithLedger(context.Background()), ExecuteInput{
		TenantUUID: uuid.NewString(), Version: uuid.NewString(), TraceID: uuid.NewString(), Manifest: evidenceDefinition(),
		Context: map[string]any{"locale": "en-US"}, Payload: map[string]any{"message": "test a:17; b:80"},
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

func fixtureFactsInventory(schema map[string]any) string {
	properties := schema["properties"].(map[string]any)["facts"].(map[string]any)["properties"].(map[string]any)
	facts := map[string]any{}
	for key, value := range properties {
		labels := value.(map[string]any)["properties"].(map[string]any)["label"].(map[string]any)["enum"].([]string)
		facts[key] = map[string]any{"label": labels[0], "scope": "campaign"}
	}
	b, _ := json.Marshal(map[string]any{"schema": evidence.GenericFactInventorySchema, "facts": facts})
	return string(b)
}

func TestManifestEvidenceKeepsFullSaaSReviewAndCalculatesDeclaredRatios(t *testing.T) {
	article, err := os.ReadFile("../../../tests/fixtures/agent_runtime/marketing_review_saas.txt")
	require.NoError(t, err)
	policyBytes, err := os.ReadFile("../../../cmd/database/seed/locales/marketing_calculation_policy.json")
	require.NoError(t, err)
	var policy map[string]any
	require.NoError(t, json.Unmarshal(policyBytes, &policy))
	manifest := evidenceDefinition()
	manifest["executor"].(map[string]any)["prompt_template_i18n"] = map[string]any{"zh-CN": "test"}
	manifest["executor"].(map[string]any)["calculation_policy"] = policy
	reviewBytes, err := os.ReadFile("../../../cmd/database/seed/locales/marketing_review_policy.json")
	require.NoError(t, err)
	var review map[string]any
	require.NoError(t, json.Unmarshal(reviewBytes, &review))
	manifest["executor"].(map[string]any)["review_policy"] = review
	calls := 0
	executor := NewManifestExecutor(ManifestExecutorOptions{LLM: func(_ context.Context, in ManifestLLMInvocation) (string, error) {
		calls++
		if calls == 1 {
			return fixtureFactsInventory(in.ResponseSchema), nil
		}
		if calls == 2 {
			dataSchema := in.ResponseSchema["properties"].(map[string]any)["data"].(map[string]any)
			props := dataSchema["properties"].(map[string]any)
			require.ElementsMatch(t, []string{"spend", "gmv", "incremental_gmv", "reported_roi", "reported_incremental_roi", "reported_repeat_rate", "unit_price_lower", "unit_price_upper", "inactivity_window", "campaign_duration", "repeat_window", "reported_sms_roi", "reported_feed_roi", "reported_feed_ctr", "reported_feed_order_rate"}, dataSchema["required"])
			data := map[string]any{}
			for key, value := range props {
				refs := value.(map[string]any)["properties"].(map[string]any)["token_ref"].(map[string]any)["enum"].([]string)
				require.Len(t, refs, 1, key)
				declaredScope := value.(map[string]any)["properties"].(map[string]any)["scope"].(map[string]any)["const"]
				data[key] = map[string]any{"scope": declaredScope, "token_ref": refs[0]}
			}
			b, _ := json.Marshal(map[string]any{"schema": evidence.ExtractionSchema, "data": data})
			return string(b), nil
		}
		require.Equal(t, "test", in.Payload["skill_instruction"])
		context, _ := json.Marshal(in.Payload["source_context"])
		require.Contains(t, string(context), "存量客户")
		require.Contains(t, string(context), "围观流量")
		require.NotContains(t, string(context), "34.2")
		require.Len(t, in.Payload["allowed_hypotheses"], 4)
		require.Len(t, in.Payload["allowed_actions"], 4)
		b, _ := json.Marshal(evidence.Notes{Schema: evidence.NotesSchema, Hypotheses: in.Payload["allowed_hypotheses"].([]string), Gaps: in.Payload["allowed_gaps"].([]string), Actions: in.Payload["allowed_actions"].([]string)})
		return string(b), nil
	}})
	ctx := evidence.WithLedger(context.Background())
	out, err := executor.Execute(ctx, ExecuteInput{SkillID: "marketing.review_summarize", TenantUUID: uuid.NewString(), Version: uuid.NewString(), TraceID: uuid.NewString(), Manifest: manifest, Context: map[string]any{"locale": "zh-CN"}, Payload: map[string]any{"message": string(article)}})
	require.NoError(t, err)
	require.NoError(t, evidence.Verify(ctx, out["response_envelope"]))
	presentation := out["response_envelope"].(map[string]any)["presentation"].(map[string]any)
	reported := presentation["reported"].([]any)
	values := []string{}
	for _, raw := range reported {
		values = append(values, raw.(map[string]any)["value"].(string))
	}
	byKey := map[string]map[string]any{}
	for _, raw := range reported {
		item := raw.(map[string]any)
		byKey[item["key"].(string)] = item
	}
	require.Len(t, reported, 15)
	require.Equal(t, "客单价下限", byKey["unit_price_lower"]["label"])
	require.Equal(t, "6000", byKey["unit_price_lower"]["value"])
	require.Equal(t, "2.4", byKey["unit_price_upper"]["value"])
	require.Equal(t, "未续费或未升级回溯期", byKey["inactivity_window"]["label"])
	require.Equal(t, "二次购买观察期", byKey["repeat_window"]["label"])
	require.Equal(t, "短信渠道", byKey["reported_sms_roi"]["scope"])
	require.Equal(t, "信息流渠道", byKey["reported_feed_roi"]["scope"])
	for _, value := range []string{"6000", "2.4", "34.2", "46.2", "1.35", "3.37", "29.0", "27.8", "0.65", "4.8", "1.2", "0.08"} {
		require.Contains(t, values, value)
	}
	require.Len(t, presentation["computed"], 2)
	require.Len(t, presentation["conflicts"], 1)
}

func TestManifestEvidenceRejectsOmissionAndInventedInputGaps(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stage  int
		reason string
	}{
		{"omitted_fact", 1, "generic_inventory_incomplete"},
		{"omitted_calculation_operand", 2, "source_selection_incomplete"},
		{"invented_gap", 3, "missing_inputs_unacknowledged"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			executor := NewManifestExecutor(ManifestExecutorOptions{LLM: func(_ context.Context, in ManifestLLMInvocation) (string, error) {
				calls++
				switch calls {
				case 1:
					if tc.stage == 1 {
						return `{"schema":"powerx.agent.evidence-facts/v2","facts":{}}`, nil
					}
					return fixtureFactsInventory(in.ResponseSchema), nil
				case 2:
					if tc.stage == 2 {
						return `{"schema":"powerx.agent.evidence-source/v1","data":{"a":{"scope":"campaign","token_ref":"token_0"}}}`, nil
					}
					return `{"schema":"powerx.agent.evidence-source/v1","data":{"a":{"scope":"campaign","token_ref":"token_0"},"b":{"scope":"campaign","token_ref":"token_1"}}}`, nil
				default:
					return `{"schema":"powerx.agent.evidence-notes/v1","hypotheses":[],"gaps":["a calculation definition not supplied"],"actions":[]}`, nil
				}
			}})
			_, err := executor.Execute(evidence.WithLedger(context.Background()), ExecuteInput{SkillID: "customer.any", TenantUUID: uuid.NewString(), Version: uuid.NewString(), TraceID: uuid.NewString(), Manifest: evidenceDefinition(), Context: map[string]any{"locale": "en-US"}, Payload: map[string]any{"message": "test a:17; b:80"}})
			require.ErrorContains(t, err, tc.reason)
			require.Equal(t, tc.stage, calls)
		})
	}
}

func TestMergeGenericFactsKeepsSameValueInDifferentSourceContexts(t *testing.T) {
	tokens, err := evidence.TokenizeGenericSources(map[string]any{"message": "短信渠道ROI 0.65，信息流渠道ROI 0.65"}, []string{"/message"})
	require.NoError(t, err)
	inventory := fixtureFactsInventory(evidence.GenericFactInventoryJSONSchema(tokens))
	var selected evidence.GenericFactInventory
	require.NoError(t, json.Unmarshal([]byte(inventory), &selected))
	facts, err := evidence.ResolveGenericFactInventory(selected, tokens)
	require.NoError(t, err)
	require.Len(t, mergeGenericFacts(nil, facts), 2)
	require.Len(t, mergeGenericFacts(facts[:1], facts), 2)
}

func TestManifestReviewRejectsUnsupportedNarrativeEvenWithValidCalculations(t *testing.T) {
	for _, hypotheses := range [][]string{{"Unproven improvement without a baseline"}, {}} {
		manifest := evidenceDefinition()
		manifest["executor"].(map[string]any)["review_policy"] = evidence.ReviewPolicy{Schema: evidence.ReviewPolicySchema, Rules: []evidence.ReviewRule{{
			Key: "source_boundary", Target: "hypotheses", TextI18n: map[string]string{"en-US": "The source remains independently unverified"},
			EvidenceAllI18n: map[string][]string{"en-US": {}}, WhenAnyFields: []string{"a"}, WhenAnyConflicts: []string{}, WhenAnyMissing: []string{},
		}}}
		calls := 0
		executor := NewManifestExecutor(ManifestExecutorOptions{LLM: func(_ context.Context, in ManifestLLMInvocation) (string, error) {
			calls++
			if calls == 1 {
				return fixtureFactsInventory(in.ResponseSchema), nil
			}
			if calls == 2 {
				return `{"schema":"powerx.agent.evidence-source/v1","data":{"a":{"scope":"s","token_ref":"token_0"},"b":{"scope":"s","token_ref":"token_1"}}}`, nil
			}
			body, err := json.Marshal(evidence.Notes{Schema: evidence.NotesSchema, Hypotheses: hypotheses, Gaps: []string{}, Actions: []string{}})
			return string(body), err
		}})
		_, err := executor.Execute(evidence.WithLedger(context.Background()), ExecuteInput{TenantUUID: uuid.NewString(), Version: uuid.NewString(), TraceID: uuid.NewString(), Manifest: manifest, Context: map[string]any{"locale": "en-US"}, Payload: map[string]any{"message": "test a:17; b:80"}})
		require.ErrorContains(t, err, "skill.evidence.notes: evidence.review_notes_")
		require.Equal(t, 3, calls)
	}
}
