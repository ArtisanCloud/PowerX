//go:build ignore

package agentmodelhubcontract

import (
	"context"
	"net"
	"testing"

	agentmodelhubv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent_model_hub/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	agentmodelhubgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/agent_model_hub"
	ammatestenv "github.com/ArtisanCloud/PowerX/tests/agent_model_hub/testenv"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestProviderGRPCContract(t *testing.T) {
	env := ammatestenv.New(t)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer()
	agentmodelhubv1.RegisterAgentModelHubServiceServer(server, agentmodelhubgrpc.NewServer(&shared.Deps{DB: env.DB}))

	go func() {
		_ = server.Serve(lis)
	}()
	t.Cleanup(func() {
		server.GracefulStop()
	})

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	client := agentmodelhubv1.NewAgentModelHubServiceClient(conn)

	regResp, err := client.RegisterProvider(context.Background(), &agentmodelhubv1.RegisterProviderRequest{
		Profile: &agentmodelhubv1.ProviderProfileInput{
			Name:            "anthropic",
			Capabilities:    []string{"llm"},
			PrimaryEndpoint: "https://api.anthropic.com",
			Regions:         []string{"us"},
			TenantWhitelist: []*agentmodelhubv1.TenantRef{
				{TenantId: "demo", Environment: "staging"},
			},
			Credentials: map[string]string{"api_key": "test"},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, regResp.GetProfile().GetProviderId())

	_, err = client.ValidateProvider(context.Background(), &agentmodelhubv1.ValidateProviderRequest{
		ProviderId: regResp.GetProfile().GetProviderId(),
		Suite:      "full",
	})
	require.NoError(t, err)

	_, err = client.PublishProvider(context.Background(), &agentmodelhubv1.PublishProviderRequest{
		ProviderId:      regResp.GetProfile().GetProviderId(),
		RolloutStrategy: agentmodelhubv1.RolloutStrategy_ROLLOUT_STRATEGY_FULL,
	})
	require.NoError(t, err)
}
