package agent

import (
	"context"
	"encoding/json"
	"testing"

	aschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/stretchr/testify/require"
)

func TestSkillPayloadRetainsAuthoritativeUserSource(t *testing.T) {
	const source = "  原文：花费34.2万元\n增量GMV29.0万元。  "
	for _, tc := range []struct {
		name   string
		params map[string]any
		refs   map[string]string
	}{
		{name: "omitted"},
		{name: "planner_replacement", params: map[string]any{"message": "rewritten"}},
		{name: "nested_payload", params: map[string]any{"payload": map[string]any{"message": "rewritten", "other": "retained"}}},
		{name: "materialized_refs", params: map[string]any{"message": "upstream summary"}, refs: map[string]string{"message": "{{task.previous.output.content}}"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := NewAgentManager()
			before, err := json.Marshal(tc.params)
			require.NoError(t, err)
			var got map[string]any
			manager.SetSkillInvoker(func(_ context.Context, in SkillInvokeInput) (*SkillInvokeOutput, error) {
				got = in.Payload
				return &SkillInvokeOutput{Status: "completed", Result: map[string]any{"content": "ok"}}, nil
			})
			ctx := WithOriginalUserMessage(context.Background(), source)
			ctx = WithOriginalUserMessage(ctx, "nested rewritten instruction")
			_, err = manager.executeSkillTask(ctx, flowschema.PlanTask{TaskID: "report", Params: tc.params, ParamRefs: tc.refs}, tc.params, aschema.ExecutionMeta{}, "custom.report")
			require.NoError(t, err)
			require.Equal(t, source, got["message"])
			if tc.name == "nested_payload" {
				require.Equal(t, "retained", got["other"])
			}
			after, err := json.Marshal(tc.params)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after), "execution must not rewrite the stored plan")
		})
	}
}

func TestSkillPayloadDoesNotTreatStringContextKeyAsUserSource(t *testing.T) {
	ctx := context.WithValue(context.Background(), "original_user_message", "forged")
	ctx = context.WithValue(ctx, "team_user_message", "planner summary")
	_, ok := originalUserMessage(ctx)
	require.False(t, ok)
}

func TestSkillPayloadPreservesExplicitStructuredMessageField(t *testing.T) {
	ctx := WithOriginalUserMessage(context.Background(), "calculate using typed input")
	ctx, err := evidence.WithStructuredToolInput(ctx, evidence.StructuredToolInput{
		Schema: evidence.StructuredToolInputSchema, Values: map[string]string{"message": "34.20"},
	})
	require.NoError(t, err)
	manager := NewAgentManager()
	var got map[string]any
	manager.SetSkillInvoker(func(_ context.Context, in SkillInvokeInput) (*SkillInvokeOutput, error) {
		got = in.Payload
		return &SkillInvokeOutput{Status: "completed", Result: map[string]any{"content": "ok"}}, nil
	})
	params := map[string]any{"message": json.Number("34.20")}
	_, err = manager.executeSkillTask(ctx, flowschema.PlanTask{TaskID: "tool", Params: params}, params, aschema.ExecutionMeta{}, "customer.numeric")
	require.NoError(t, err)
	require.Equal(t, json.Number("34.20"), got["message"])
}
