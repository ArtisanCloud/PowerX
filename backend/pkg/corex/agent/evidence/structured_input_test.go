package evidence

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestStructuredInputFreezesOriginalDecimalAndPrincipal(t *testing.T) {
	ctx := reqctx.WithEnv(reqctx.WithUserUUID(reqctx.WithTenantUUID(context.Background(), uuid.NewString()), uuid.NewString()), "dev")
	value := "123456789012345678901234567890123456789012345678901234567890.125"
	declared := StructuredToolInput{Schema: StructuredToolInputSchema, Values: map[string]string{"length_cm": value}}
	frozen, err := WithStructuredToolInput(ctx, declared)
	require.NoError(t, err)
	declared.Values["length_cm"] = "999"
	exported := StructuredToolInputDeclaration(frozen)
	exported.Values["length_cm"] = "888"
	values, err := TrustedToolInputValues(WithToolAgentID(frozen, 9))
	require.NoError(t, err)
	require.Equal(t, json.Number(value), values["length_cm"])
	_, err = TrustedToolInputValues(reqctx.WithUserUUID(frozen, uuid.NewString()))
	require.ErrorContains(t, err, "evidence.source_invalid")
	// JSON 和同名普通 context key 都不能带出私有来源权威。
	wire, err := json.Marshal(StructuredToolInputDeclaration(frozen))
	require.NoError(t, err)
	_, err = TrustedToolInputValues(context.WithValue(ctx, "agent_structured_tool_input", string(wire)))
	require.ErrorContains(t, err, "evidence.source_invalid")
}

func TestStructuredInputRejectsMalformedAndCoercedValues(t *testing.T) {
	for _, v := range []any{
		map[string]any{"schema": StructuredToolInputSchema, "values": map[string]any{"length_cm": 1}},
		StructuredToolInput{Schema: StructuredToolInputSchema, Values: map[string]string{"length_cm": "1e3"}},
		StructuredToolInput{Schema: StructuredToolInputSchema, Values: map[string]string{"length_cm": "01"}},
		map[string]any{"schema": StructuredToolInputSchema, "values": map[string]string{"length_cm": "1"}, "trusted": true},
	} {
		_, err := WithStructuredToolInput(context.Background(), v)
		require.ErrorContains(t, err, "evidence.source_invalid")
	}
}
