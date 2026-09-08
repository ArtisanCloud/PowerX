package runtime

import (
	"testing"

	agenttrace "github.com/ArtisanCloud/PowerX/internal/service/agent_trace"
	"github.com/stretchr/testify/require"
)

func TestTraceMetaMapIncludesTenantUUIDForHistoryAndSSE(t *testing.T) {
	trace := &traceRuntime{meta: agenttrace.AgentRunMeta{
		TenantUUID: "tenant-uuid",
		TraceID:   "trace-id",
		RunID:     "run-id",
		SessionID: "session-id",
		MessageID: "message-id",
	}}

	meta := traceMetaMap(trace)
	require.Equal(t, "tenant-uuid", meta["tenant_uuid"])
	require.Equal(t, "trace-id", meta["trace_id"])
	require.Equal(t, "run-id", meta["run_id"])
}
