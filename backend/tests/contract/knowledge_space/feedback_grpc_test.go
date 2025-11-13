package knowledge_space_contract

import (
	"context"
	"net"
	"testing"
	"time"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
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

	resp, err := client.SubmitFeedback(ctx, &knowledgev1.FeedbackRequest{
		SpaceId:      space.UUID.String(),
		Severity:     "critical",
		IssueType:    "compliance",
		Notes:        "PII detected",
		LinkedChunks: []string{uuid.NewString()},
		ReportedBy:   "sre@powerx.local",
	})
	require.NoError(t, err)
	require.NotNil(t, resp.GetCase())
	require.Equal(t, "in_progress", resp.GetCase().GetStatus())

	listResp, err := client.ListFeedbackCases(ctx, &knowledgev1.ListFeedbackCasesRequest{
		SpaceId: space.UUID.String(),
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(listResp.GetCases()), 1)
}
