package evidence

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestPublishedReportSchemaMatchesRuntime(t *testing.T) {
	if os.Getenv("POWERX_PRINT_EVIDENCE_SCHEMA") == "1" {
		b, _ := json.MarshalIndent(ReportJSONSchema(), "", "  ")
		t.Log(string(b))
		return
	}
	raw, err := os.ReadFile("../../../../config/agent/response_envelope.v4.schema.json")
	require.NoError(t, err)
	expected, _ := json.Marshal(ReportJSONSchema())
	require.JSONEq(t, string(expected), string(raw))
}

func TestDraftRequiresExplicitPrecisionAndPercent(t *testing.T) {
	b, _ := json.Marshal(testDraft())
	var raw map[string]any
	require.NoError(t, json.Unmarshal(b, &raw))
	delete(raw["calculations"].([]any)[0].(map[string]any), "percent")
	b, _ = json.Marshal(raw)
	var d Draft
	require.Error(t, Decode(b, &d))
}

func TestSourceQuoteCannotTruncateOriginalNumericToken(t *testing.T) {
	for _, tc := range [][3]string{{"1100", "100", "100"}, {"27.8%", "27.8", "27.8"}, {"27.8％", "27.8", "27.8"}, {"3.37", "3.3", "3.3"}} {
		require.False(t, containsQuotedNumericToken(tc[0], tc[1], tc[2]))
	}
	require.True(t, containsQuotedNumericToken("ROI 3.37。", "3.37", "3.37"))
}
