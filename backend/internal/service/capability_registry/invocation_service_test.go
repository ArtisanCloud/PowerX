package capability_registry

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/utils/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	mediapb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/media/v1"
)

func TestInvocationServiceInvokeREST(t *testing.T) {
	t.Parallel()
	testutil.SkipIfNoLocalListener(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/media/assets", r.URL.Path)
		require.Equal(t, "Bearer demo", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","items":[1,2,3]}`))
	}))
	defer server.Close()

	svc := &InvocationService{
		httpClient:  server.Client(),
		httpBaseURL: server.URL,
	}

	payload := restInvokePayload{
		Method:   http.MethodGet,
		Endpoint: "/media/assets",
		Headers: map[string]string{
			"Authorization": "Bearer demo",
		},
	}
	result, err := svc.invokeREST(context.Background(), payload, "trace-http")
	require.NoError(t, err)
	require.Equal(t, "ok", result["status"])
}

func TestInvocationServiceInvokeGRPC(t *testing.T) {
	t.Parallel()

	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	mediapb.RegisterMediaAssetAdminServiceServer(grpcServer, &stubMediaServer{})
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(func(ctx context.Context, s string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	svc := &InvocationService{
		grpcConn: conn,
	}

	payload := grpcInvokePayload{
		Service: "powerx.media.v1.MediaAssetAdminService",
		Method:  "ListMediaAssets",
		Body: map[string]interface{}{
			"page":      1,
			"page_size": 2,
		},
	}
	result, err := svc.invokeGRPC(context.Background(), payload)
	require.NoError(t, err)
	dataField, ok := result["data"].(map[string]interface{})
	require.True(t, ok)
	items, ok := dataField["items"].([]interface{})
	require.True(t, ok)
	require.Len(t, items, 1)
}

type stubMediaServer struct {
	mediapb.UnimplementedMediaAssetAdminServiceServer
}

func (s *stubMediaServer) ListMediaAssets(ctx context.Context, req *mediapb.ListMediaAssetsRequest) (*mediapb.ListMediaAssetsResponse, error) {
	return &mediapb.ListMediaAssetsResponse{
		Data: &mediapb.ListMediaAssetsData{
			Items: []*mediapb.MediaAsset{
				{Uuid: "asset-1"},
			},
		},
	}, nil
}
