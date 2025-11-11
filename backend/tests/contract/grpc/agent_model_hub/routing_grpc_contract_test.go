//go:build ignore

package agentmodelhubcontract

import (
	"context"
	"net"
	"testing"

	agentmodelhubv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent_model_hub/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	agentmodelhubgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/agent_model_hub"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	ammatestenv "github.com/ArtisanCloud/PowerX/tests/agent_model_hub/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestRoutingGRPCContract(t *testing.T) {
	env := ammatestenv.New(t)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	agentmodelhubv1.RegisterAgentModelHubServiceServer(server, agentmodelhubgrpc.NewServer(&shared.Deps{DB: env.DB}))

	go func() { _ = server.Serve(lis) }()
	t.Cleanup(server.GracefulStop)

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	client := agentmodelhubv1.NewAgentModelHubServiceClient(conn)

	policyID := uuid.NewString()
	resp, err := client.UpsertRoutingPolicy(context.Background(), &agentmodelhubv1.UpsertRoutingPolicyRequest{
		Policy: &agentmodelhubv1.RoutingPolicyInput{
			TenantScope: "tenant-contract",
			Rules: []*agentmodelhubv1.RoutingRule{
				{
					TaskPattern: "chat",
					Candidates: []*agentmodelhubv1.ProviderWeight{
						{ProviderId: "provider-primary", Weight: 1},
						{ProviderId: "provider-backup", Weight: 0.5},
					},
				},
			},
			FallbackChain: []*agentmodelhubv1.ProviderWeight{{ProviderId: "provider-backup"}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp.GetPolicy())

	_, err = client.UpdateRoutingPolicyStatus(context.Background(), &agentmodelhubv1.UpdateRoutingPolicyStatusRequest{
		TenantScope:  "tenant-contract",
		TargetStatus: agentmodelhubv1.PolicyStatus_POLICY_STATUS_ACTIVE,
	})
	require.NoError(t, err)

	decisionResp, err := client.RouteTask(context.Background(), &agentmodelhubv1.RouteTaskRequest{
		TenantId:    "tenant-contract",
		TaskContext: map[string]string{"taskType": "chat"},
	})
	require.NoError(t, err)
	require.Equal(t, "provider-primary", decisionResp.GetPrimaryProviderId())

	_, err = client.RollbackRoutingPolicy(context.Background(), &agentmodelhubv1.RollbackRoutingPolicyRequest{
		TenantScope:   "tenant-contract",
		TargetVersion: resp.GetPolicy().GetVersion(),
	})
	require.NoError(t, err)

	_, err = client.ToggleSafeMode(context.Background(), &agentmodelhubv1.ToggleSafeModeRequest{
		TenantScope: "tenant-contract",
		Enabled:     true,
		TtlSeconds:  60,
		Reason:      "contract-test",
	})
	require.NoError(t, err)

	_ = policyID // placeholder to ensure uuid import used when extending tests
}
