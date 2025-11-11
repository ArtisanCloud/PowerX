//go:build ignore

package plugin_release

import (
	"context"
	"net"
	"testing"
	"time"

	pb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/plugin_release/v1"
	grpcserver "github.com/ArtisanCloud/PowerX/internal/transport/grpc/plugin_release"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	loggerconfig "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

const grpcBufferSize = 1024 * 1024

func TestPluginReleaseGRPC_LocalInstallLifecycle(t *testing.T) {
	logger.InitGlobalLogger(&loggerconfig.LogConfig{Level: "error"})

	deps, _ := setupPluginReleaseDeps(t)

	listener := bufconn.Listen(grpcBufferSize)
	t.Cleanup(func() { _ = listener.Close() })

	server := grpc.NewServer()
	grpcserver.RegisterServer(server, deps)
	go func() {
		if err := server.Serve(listener); err != nil {
			t.Errorf("gRPC server failed: %v", err)
		}
	}()
	t.Cleanup(server.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		"bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := pb.NewPluginReleaseServiceClient(conn)

	callCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer test-token")
	startResp, err := client.StartLocalInstall(callCtx, &pb.StartLocalInstallRequest{
		TenantId:     "101",
		DeveloperId:  2025,
		ArtifactUri:  "s3://bucket/artifact.zip",
		FeatureFlags: []string{"beta_ui"},
		ResetCache:   true,
	})
	require.NoError(t, err)
	require.Equal(t, "101", startResp.GetTenantId())
	require.Equal(t, uint64(2025), startResp.GetDeveloperId())
	require.Equal(t, models.LocalInstallStatusInProgress, startResp.GetStatus())

	getResp, err := client.GetLocalInstallSession(callCtx, &pb.GetLocalInstallSessionRequest{
		SessionId: startResp.GetSessionId(),
		TenantId:  "101",
	})
	require.NoError(t, err)
	require.Equal(t, startResp.GetSessionId(), getResp.GetSessionId())
	require.Equal(t, models.LocalInstallStatusInProgress, getResp.GetStatus())

	stream, err := client.PushHotReload(callCtx)
	require.NoError(t, err)
	require.NoError(t, stream.Send(&pb.HotReloadChunk{
		SessionId: startResp.GetSessionId(),
		Sequence:  1,
		Content:   []byte("payload"),
		Changelog: "apply change",
	}))
	require.NoError(t, stream.Send(&pb.HotReloadChunk{
		SessionId: startResp.GetSessionId(),
		Sequence:  2,
		Eof:       true,
	}))
	ack, err := stream.CloseAndRecv()
	require.NoError(t, err)
	require.Equal(t, startResp.GetSessionId(), ack.GetSessionId())
	require.Equal(t, int64(2), ack.GetAppliedSequence())
	require.Equal(t, "completed", ack.GetStatus())

	stopResp, err := client.StopLocalInstall(callCtx, &pb.StopLocalInstallRequest{
		SessionId: startResp.GetSessionId(),
		Force:     false,
	})
	require.NoError(t, err)
	require.Equal(t, models.LocalInstallStatusSuccess, stopResp.GetStatus())

	finalResp, err := client.GetLocalInstallSession(callCtx, &pb.GetLocalInstallSessionRequest{
		SessionId: startResp.GetSessionId(),
	})
	require.NoError(t, err)
	require.Equal(t, models.LocalInstallStatusSuccess, finalResp.GetStatus())
}
