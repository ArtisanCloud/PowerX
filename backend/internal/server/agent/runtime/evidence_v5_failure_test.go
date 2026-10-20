package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	trace "github.com/ArtisanCloud/PowerX/internal/service/agent_trace"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestFinalEvidenceFailureHasActualTraceNodeAndSameRealtimeCode(t *testing.T) {
	tenant, user := uuid.NewString(), uuid.NewString()
	ctx := reqctx.WithTraceID(reqctx.WithUserUUID(reqctx.WithTenantUUID(context.Background(), tenant), user), "trace_v5")
	ctx = context.WithValue(context.WithValue(context.WithValue(ctx, "agent_numeric_id", uint64(1)), "session_id", "session_v5"), "message_id", "message_v5")
	sink := trace.NewLocalSink(t.TempDir())
	logger := trace.NewLogger(trace.Config{Enabled: true, LocalEnabled: true}, sink)
	_, err := logger.StartRun(ctx, trace.AgentRunMeta{RunID: "run_v5", TraceID: "trace_v5", TenantUUID: tenant, UserUUID: user, AgentID: "1", SessionID: "session_v5", MessageID: "message_v5"})
	require.NoError(t, err)
	manager := agent.NewAgentManager()
	manager.SetAgentTraceLogger(logger)
	err = (&ManagerTaskInvoker{Manager: manager}).failFinalEvidence(ctx, "run_v5", "summary", errors.New("forged execution ref"))
	report, readErr := logger.BuildReport(ctx, trace.AgentReportQuery{TenantUUID: tenant, RunID: "run_v5", TraceID: "trace_v5", SessionID: "session_v5", MessageID: "message_v5"})
	require.NoError(t, readErr)
	require.Len(t, report.Nodes, 1)
	require.Equal(t, trace.EventStatusError, report.Nodes[0].PhaseStatus)
	require.Equal(t, "agent.response_contract_invalid", report.Nodes[0].ErrorCode)
	require.Equal(t, "final_validation", report.Nodes[0].OutputSummary["failure_stage"])
	realtime := &captureSink{}
	emitFinalEvidenceFailure(ctx, realtime, "plan", err)
	emitAgentRunFailure(ctx, realtime, "plan", "final.response_contract_invalid", "", err, "")
	require.Equal(t, []string{dto.EventNodeEnd, dto.EventFinal, dto.EventError, dto.EventEnd}, realtime.events)
	final := realtime.data[1].(map[string]any)
	require.Equal(t, false, final["success"])
	require.NotContains(t, final["data"], "response_envelope")
	errorPayload := realtime.data[2].(map[string]any)
	require.Equal(t, "agent.response_contract_invalid", errorPayload["code"])
	require.Equal(t, "final_validation", errorPayload["failure_stage"])
	require.Equal(t, RuntimeOutcomeFailed, errorPayload["outcome"])
	meta := extractAssistantTraceMeta(final)
	require.Equal(t, "agent.response_contract_invalid", meta["reason_code"])
	require.Equal(t, "final_validation", meta["failure_stage"])
	durableMeta := datatypes.JSONMap{}
	projectTaskFailureMeta(durableMeta, []agent_run.TaskSnapshot{{TaskID: "summary", Status: "failed", ReasonCode: "agent.response_contract_invalid", FailureStage: "final_validation"}})
	require.Equal(t, meta["reason_code"], durableMeta["reason_code"])
	require.Equal(t, meta["failure_stage"], durableMeta["failure_stage"])
}
