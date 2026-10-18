package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToolCallingDecisionDistinguishesEmptySelectionFromInvalidOutput(t *testing.T) {
	for _, raw := range []string{
		`{}`,
		`{"tool_calls":null}`,
		`{"tool_calls":{}}`,
		`## 工具调用决策
- 选择工具: customer.summary
- 参数: {"活动主体":"SaaS产品","周期":"28天"}
- 置信度: 0.95`,
	} {
		_, err := parseToolCallingDecision(raw)
		require.Error(t, err, "invalid output must not become a successful empty selection: %s", raw)
	}
	empty, err := parseToolCallingDecision(`{"tool_calls":[]}`)
	require.NoError(t, err)
	require.Empty(t, empty.ToolCalls)
	selected, err := parseToolCallingDecision("```json\n{\"tool_calls\":[{\"name\":\"customer.summary\",\"args\":{},\"confidence\":1}]}\n```")
	require.NoError(t, err)
	require.Len(t, selected.ToolCalls, 1)
	require.Equal(t, "customer.summary", selected.ToolCalls[0].Name)
}
