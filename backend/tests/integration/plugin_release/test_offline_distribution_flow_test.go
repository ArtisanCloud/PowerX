package pluginreleaseintegration

import (
	"context"
	"net"
	"testing"
	"time"

	pluginreleasepb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/plugin_release/v1"
	grpcserver "github.com/ArtisanCloud/PowerX/internal/transport/grpc/plugin_release"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestLegacyOfflineDistributionRPCIsDisabled(t *testing.T) {
	env := newPluginReleaseEnv(t)

	listener := bufconn.Listen(bufSize)
	t.Cleanup(func() { _ = listener.Close() })

	server := grpc.NewServer()
	grpcserver.RegisterServer(server, env.Deps)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := pluginreleasepb.NewPluginReleaseServiceClient(conn)
	stream, err := client.UploadOfflinePackage(ctx)
	require.NoError(t, err)
	_, err = stream.CloseAndRecv()
	require.Error(t, err)
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}
