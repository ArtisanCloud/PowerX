package knowledge_space_contract

import (
	"context"
	"net"
	"testing"
	"time"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestFeedbackGRPCHandlers(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)
	env.Pipeline.WithInner(nil)

	listener := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = listener.Close() })

	server := env.GRPCServer()
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := knowledgev1.NewKnowledgeSpaceAdminServiceClient(conn)
	policyID := env.SeedPolicyTemplate("grpc-feedback", "v1")
	space := env.CreateSpaceFixture("grpc-feedback-space", policyID)

	rpcCtx := knowledgeGRPCContext(t, env)
	traceID := "trace-grpc-123"
	resp, err := client.SubmitFeedback(rpcCtx, &knowledgev1.FeedbackRequest{
		SpaceId:      space.UUID.String(),
		Severity:     "critical",
		IssueType:    "compliance",
		Notes:        "PII detected",
		LinkedChunks: []string{uuid.NewString()},
		ReportedBy:   "sre@powerx.local",
		ToolTraceRef: traceID,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.GetCase())
	require.Equal(t, "in_progress", resp.GetCase().GetStatus())
	require.Equal(t, traceID, resp.GetCase().GetToolTraceRef())
	assertNoLegacyTenantProto(t, resp)

	listResp, err := client.ListFeedbackCases(rpcCtx, &knowledgev1.ListFeedbackCasesRequest{
		SpaceId: space.UUID.String(),
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(listResp.GetCases()), 1)
	assertNoLegacyTenantProto(t, listResp)

	escalateResp, err := client.EscalateFeedbackCase(rpcCtx, &knowledgev1.EscalateFeedbackCaseRequest{
		SpaceId:      space.UUID.String(),
		CaseId:       resp.GetCase().GetCaseId(),
		RequestedBy:  "sre@powerx.local",
		Reason:       "需要人工复核",
	})
	require.NoError(t, err)
	require.Equal(t, "escalated", escalateResp.GetCase().GetStatus())

	closeResp, err := client.CloseFeedbackCase(rpcCtx, &knowledgev1.CloseFeedbackCaseRequest{
		SpaceId:          space.UUID.String(),
		CaseId:           resp.GetCase().GetCaseId(),
		RequestedBy:      "sre@powerx.local",
		ResolutionNotes:  "已完成热更新",
	})
	require.NoError(t, err)
	require.Equal(t, "closed", closeResp.GetCase().GetStatus())

	exportResp, err := client.ExportFeedbackCases(rpcCtx, &knowledgev1.ExportFeedbackCasesRequest{
		SpaceId: space.UUID.String(),
		Limit:   10,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(exportResp.GetCases()), 1)
	require.NotEmpty(t, exportResp.GetExportJson())
	assertNoLegacyTenantProto(t, exportResp)

	reprocessResp, err := client.ReprocessFeedbackCase(rpcCtx, &knowledgev1.ReprocessFeedbackCaseRequest{
		SpaceId:     space.UUID.String(),
		CaseId:      resp.GetCase().GetCaseId(),
		RequestedBy: "sre@powerx.local",
	})
	require.NoError(t, err)
	require.Equal(t, "in_progress", reprocessResp.GetCase().GetStatus())

	_, err = env.Deps.KnowledgeSpace.Service.RetireSpace(context.Background(), ksvc.RetireSpaceInput{
		SpaceID: space.UUID,
	})
	require.NoError(t, err)
	_, err = client.SubmitFeedback(rpcCtx, &knowledgev1.FeedbackRequest{
		SpaceId:      space.UUID.String(),
		Severity:     "medium",
		IssueType:    "accuracy",
		Notes:        "should be blocked",
		LinkedChunks: []string{uuid.NewString()},
		ReportedBy:   "qa@powerx.local",
	})
	require.Error(t, err)
}
