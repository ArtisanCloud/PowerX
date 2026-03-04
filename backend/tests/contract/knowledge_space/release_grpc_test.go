package knowledge_space_contract

import (
	"context"
	"net"
	"testing"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

func TestReleaseGRPCFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	server := env.GRPCServer()
	lis := bufconn.Listen(1024 * 1024)
	go func() {
		_ = server.Serve(lis)
	}()
	t.Cleanup(func() { server.Stop() })

	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithInsecure(),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	client := knowledgev1.NewKnowledgeSpaceAdminServiceClient(conn)

	ctx := knowledgeGRPCContext(t, env)
	upsertResp, err := client.UpsertReleasePolicy(ctx, &knowledgev1.UpsertReleasePolicyRequest{
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
	assertNoLegacyTenantProto(t, upsertResp)

	publishResp, err := client.PublishRelease(ctx, &knowledgev1.PublishReleaseRequest{
		PolicyId:    upsertResp.GetPolicyId(),
		VersionId:   "ver-2025.02",
		RequestedBy: "qa@powerx.io",
	})
	require.NoError(t, err)
	require.NotEmpty(t, publishResp.GetBatchToken())
	assertNoLegacyTenantProto(t, publishResp)

	promoteResp, err := client.PromoteRelease(ctx, &knowledgev1.PromoteReleaseRequest{
		PolicyId:    upsertResp.GetPolicyId(),
		VersionId:   "ver-2025.02",
		BatchToken:  publishResp.GetBatchToken(),
		RequestedBy: "ops@powerx.io",
	})
	require.NoError(t, err)
	require.NotEmpty(t, promoteResp.GetNextBatchToken())
	assertNoLegacyTenantProto(t, promoteResp)

	rollbackResp, err := client.RollbackRelease(ctx, &knowledgev1.RollbackReleaseRequest{
		PolicyId:    upsertResp.GetPolicyId(),
		VersionId:   "ver-2025.02",
		Reason:      "metrics breached",
		RequestedBy: "ops@powerx.io",
	})
	require.NoError(t, err)
	require.Equal(t, "rolled_back", rollbackResp.GetStatus())
	assertNoLegacyTenantProto(t, rollbackResp)
}
