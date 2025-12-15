package knowledge_space_contract

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestProvisioningGRPCFlow(t *testing.T) {
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

	policyID := env.SeedPolicyTemplate("grpc-template", "v1")

	rpcCtx := knowledgeGRPCContext(t, env)
	createResp, err := client.CreateKnowledgeSpace(rpcCtx, &knowledgev1.CreateKnowledgeSpaceRequest{
		TenantUuid:              env.TenantUUID().String(),
		Name:                    "grpc-space",
		DepartmentCode:          "OPS-GRPC",
		QuotaCpu:                4,
		QuotaStorageGb:          180,
		PolicyTemplateVersionId: fmt.Sprint(policyID),
		FeatureFlags:            []string{"ingestion.dual-chunk"},
	})
	require.NoError(t, err)
	require.NotEmpty(t, createResp.GetSpace().GetSpaceId())
	require.Equal(t, "pending_iam", createResp.GetSpace().GetStatus())
	assertNoLegacyTenantProto(t, createResp)

	spaceID := createResp.GetSpace().GetSpaceId()

	updateResp, err := client.UpdateKnowledgeSpace(rpcCtx, &knowledgev1.UpdateKnowledgeSpaceRequest{
		SpaceId:                 spaceID,
		QuotaCpu:                6,
		QuotaStorageGb:          240,
		PolicyTemplateVersionId: fmt.Sprint(policyID),
		Status:                  "active",
	})
	require.NoError(t, err)
	require.Equal(t, "active", updateResp.GetSpace().GetStatus())
	require.Equal(t, uint32(6), updateResp.GetSpace().GetQuotaCpu())
	assertNoLegacyTenantProto(t, updateResp)

	retireResp, err := client.RetireKnowledgeSpace(rpcCtx, &knowledgev1.RetireKnowledgeSpaceRequest{
		SpaceId: spaceID,
		Reason:  "grpc sunset",
	})
	require.NoError(t, err)
	require.Equal(t, "retired", retireResp.GetSpace().GetStatus())
	assertNoLegacyTenantProto(t, retireResp)
}
