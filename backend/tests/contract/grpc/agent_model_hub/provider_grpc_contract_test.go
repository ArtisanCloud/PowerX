//go:build ignore

package agentmodelhubcontract

import (
	"context"
	"net"
	"strings"
	"testing"

	agentmodelhubv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent_model_hub/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	agentmodelhubgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/agent_model_hub"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	ammatestenv "github.com/ArtisanCloud/PowerX/tests/agent_model_hub/testenv"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func TestProviderGRPCContract(t *testing.T) {
	env := ammatestenv.New(t)
	env.MustInsertTenant(9001, ammatestenv.AgentModelHubTenantUUID)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	unary := func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		tenantUUID := ammatestenv.AgentModelHubTenantUUID
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if values := md.Get("x-tenant-uuid"); len(values) > 0 {
				if trimmed := strings.TrimSpace(values[0]); trimmed != "" {
					tenantUUID = trimmed
				}
			}
		}
		ctx = reqctx.WithTenantUUID(ctx, tenantUUID)
		return handler(ctx, req)
	}
	server := grpc.NewServer(grpc.UnaryInterceptor(unary))
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

	ctx := agentModelHubContext(t, ammatestenv.AgentModelHubTenantUUID)
	regResp, err := client.RegisterProvider(ctx, &agentmodelhubv1.RegisterProviderRequest{
		Profile: &agentmodelhubv1.ProviderProfileInput{
			Name:            "anthropic",
			Capabilities:    []string{"llm"},
			PrimaryEndpoint: "https://api.anthropic.com",
			Regions:         []string{"us"},
			TenantWhitelist: []*agentmodelhubv1.TenantRef{
				{TenantUuid: ammatestenv.AgentModelHubTenantUUID, Environment: "staging"},
			},
			Credentials: map[string]string{"api_key": "test"},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, regResp.GetProfile().GetProviderId())
	assertNoAgentModelHubTenantLeak(t, regResp)

	validateResp, err := client.ValidateProvider(ctx, &agentmodelhubv1.ValidateProviderRequest{
		ProviderId: regResp.GetProfile().GetProviderId(),
		Suite:      "full",
	})
	require.NoError(t, err)
	assertNoAgentModelHubTenantLeak(t, validateResp)

	publishResp, err := client.PublishProvider(ctx, &agentmodelhubv1.PublishProviderRequest{
		ProviderId:      regResp.GetProfile().GetProviderId(),
		RolloutStrategy: agentmodelhubv1.RolloutStrategy_ROLLOUT_STRATEGY_FULL,
	})
	require.NoError(t, err)
	assertNoAgentModelHubTenantLeak(t, publishResp)
}
