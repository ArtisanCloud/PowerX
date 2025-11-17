package knowledge_space_contract

import (
	"context"
	"net"
	"testing"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestReleaseGRPCFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	server := env.GRPCServer()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go server.Serve(lis)
	t.Cleanup(func() { server.Stop() })

	conn, err := grpc.DialContext(context.Background(), lis.Addr().String(), grpc.WithInsecure())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	client := knowledgev1.NewKnowledgeSpaceAdminServiceClient(conn)

	upsertResp, err := client.UpsertReleasePolicy(context.Background(), &knowledgev1.UpsertReleasePolicyRequest{
		MatrixVersion: "v2025.02",
		PilotTenants:  []string{"demo-retail"},
		Batches: []*knowledgev1.ReleaseBatch{
			{Name: "pilot", Tenants: []string{"demo-retail"}},
			{Name: "wave-2", Tenants: []string{"demo-lite", "demo-enterprise"}},
		},
		Guardrails: map[string]string{"latency_p95": "<5m"},
		ApprovedBy: "ops@powerx.io",
	})
	require.NoError(t, err)
	require.NotEmpty(t, upsertResp.GetPolicyId())

	publishResp, err := client.PublishRelease(context.Background(), &knowledgev1.PublishReleaseRequest{
		PolicyId:    upsertResp.GetPolicyId(),
		VersionId:   "ver-2025.02",
		RequestedBy: "qa@powerx.io",
	})
	require.NoError(t, err)
	require.NotEmpty(t, publishResp.GetBatchToken())

	promoteResp, err := client.PromoteRelease(context.Background(), &knowledgev1.PromoteReleaseRequest{
		PolicyId:    upsertResp.GetPolicyId(),
		VersionId:   "ver-2025.02",
		BatchToken:  publishResp.GetBatchToken(),
		RequestedBy: "ops@powerx.io",
	})
	require.NoError(t, err)
	require.NotEmpty(t, promoteResp.GetNextBatchToken())

	rollbackResp, err := client.RollbackRelease(context.Background(), &knowledgev1.RollbackReleaseRequest{
		PolicyId:    upsertResp.GetPolicyId(),
		VersionId:   "ver-2025.02",
		Reason:      "metrics breached",
		RequestedBy: "ops@powerx.io",
	})
	require.NoError(t, err)
	require.Equal(t, "rolled_back", rollbackResp.GetStatus())
}
