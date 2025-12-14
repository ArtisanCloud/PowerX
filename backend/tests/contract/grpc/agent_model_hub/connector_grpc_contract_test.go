//go:build ignore

package agentmodelhubcontract

import (
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

func TestConnectorGRPCContract(t *testing.T) {
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

	ctx := agentModelHubContext(t, "demo-tenant")
	upsertResp, err := client.UpsertConnectorInstance(ctx, &agentmodelhubv1.UpsertConnectorInstanceRequest{
		Platform: "coze",
		Instance: &agentmodelhubv1.ConnectorInstanceInput{
			TenantUuid:           "demo-tenant",
			Region:               "us-east-1",
			OauthRef:             "vault://connectors/coze/demo",
			WebhookSigningKeyRef: "vault://connectors/coze/signing",
			MappingTemplateJson:  `{"workflow":"sync_leads"}`,
			RateLimitPerMinute:   90,
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, upsertResp.GetInstance().GetInstanceId())
	assertNoAgentModelHubTenantLeak(t, upsertResp)

	pauseResp, err := client.PauseConnectorInstance(ctx, &agentmodelhubv1.PauseConnectorInstanceRequest{
		Platform:   "coze",
		InstanceId: upsertResp.GetInstance().GetInstanceId(),
		Reason:     "observed error rate spike",
	})
	require.NoError(t, err)
	assertNoAgentModelHubTenantLeak(t, pauseResp)
}
