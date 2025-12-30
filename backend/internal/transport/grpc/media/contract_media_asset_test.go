//go:build ignore

package media

import (
	"context"
	"net"
	"testing"

	corexmediav1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/media/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
)

const bufSize = 1024 * 1024

func newMediaAssetAdminClient(t *testing.T) (corexmediav1.MediaAssetAdminServiceClient, func()) {
	t.Helper()

	listener := bufconn.Listen(bufSize)
	server := grpc.NewServer()

	go func() {
		_ = server.Serve(listener)
	}()

	dialer := func(ctx context.Context, s string) (net.Conn, error) {
		return listener.Dial()
	}

	conn, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(dialer), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	cleanup := func() {
		_ = conn.Close()
		server.Stop()
	}

	return corexmediav1.NewMediaAssetAdminServiceClient(conn), cleanup
}

func TestContractGRPCCreateMediaAsset(t *testing.T) {
	t.Parallel()

	client, cleanup := newMediaAssetAdminClient(t)
	defer cleanup()

	ctx := context.Background()

	req := &corexmediav1.CreateMediaAssetRequest{
		TenantUuid:         "tenant_a",
		OperatorId:       "op_01",
		Name:             "homepage-banner",
		Driver:           "local",
		OwnerSubjectType: "campaign",
		OwnerSubjectId:   "cmp_123",
		Tags:             []string{"banner", "homepage"},
		UploadChannel:    corexmediav1.UploadChannel_UPLOAD_CHANNEL_DIRECT,
	}

	resp, err := client.CreateMediaAsset(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Data)
	require.Equal(t, "local", resp.Data.Driver)
}

func TestContractGRPCListMediaAssets(t *testing.T) {
	t.Parallel()

	client, cleanup := newMediaAssetAdminClient(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := client.ListMediaAssets(ctx, &corexmediav1.ListMediaAssetsRequest{
		TenantUuid: "tenant_a",
		Tags:     []string{"homepage"},
		Page:     1,
		PageSize: 20,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Data)
	require.Greater(t, resp.Data.Page.Total, uint64(0))
	require.Greater(t, uint32(len(resp.Data.Items)), uint32(0))
}

func TestContractGRPCGetMediaAsset(t *testing.T) {
	t.Parallel()

	client, cleanup := newMediaAssetAdminClient(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := client.GetMediaAsset(ctx, &corexmediav1.GetMediaAssetRequest{
		TenantUuid: "tenant_a",
		Uuid:     "mas_123",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "mas_123", resp.Data.Uuid)
}

func TestContractGRPCUpdateMediaAsset(t *testing.T) {
	t.Parallel()

	client, cleanup := newMediaAssetAdminClient(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := client.UpdateMediaAsset(ctx, &corexmediav1.UpdateMediaAssetRequest{
		TenantUuid:       "tenant_a",
		Uuid:           "mas_123",
		OperatorId:     "op_01",
		Name:           proto.String("updated-banner"),
		Description:    proto.String("更新描述"),
		Tags:           []string{"homepage", "2025"},
		BusinessStatus: corexmediav1.BusinessStatus_BUSINESS_STATUS_UNDER_REVIEW.Enum(),
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "updated-banner", resp.Data.Name)
	require.Equal(t, corexmediav1.BusinessStatus_BUSINESS_STATUS_UNDER_REVIEW, resp.Data.BusinessStatus)
}

func TestContractGRPCDeleteMediaAsset(t *testing.T) {
	t.Parallel()

	client, cleanup := newMediaAssetAdminClient(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := client.DeleteMediaAsset(ctx, &corexmediav1.DeleteMediaAssetRequest{
		TenantUuid:   "tenant_a",
		Uuid:       "mas_123",
		OperatorId: "op_01",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.Data.Deleted)
}

func TestContractGRPCPresignMediaAsset(t *testing.T) {
	t.Parallel()

	client, cleanup := newMediaAssetAdminClient(t)
	defer cleanup()

	ctx := context.Background()

	resp, err := client.PresignMediaAsset(ctx, &corexmediav1.PresignMediaAssetRequest{
		TenantUuid:         "tenant_a",
		Uuid:             "mas_123",
		OperatorId:       "op_01",
		Action:           corexmediav1.PresignAction_PRESIGN_ACTION_UPLOAD,
		ExpiresInSeconds: 600,
		Method:           "PUT",
		Metadata:         map[string]string{"filename": "banner.png"},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "PUT", resp.Data.Method)
	require.Equal(t, uint32(600), resp.Data.ExpiresInSeconds)
	require.NotEmpty(t, resp.Data.Url)
}
