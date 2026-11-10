package eventfabric

import (
	"context"
	"net"
	"testing"
	"time"

	pb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/event_fabric/v1"
	channel "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/service_channel"
	"github.com/ArtisanCloud/PowerX/internal/transport/grpc/auth/middleware"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
)

type testRing struct{}

func (testRing) GetHSByKID(string) []byte { return []byte("isolated-grpc-authentication-test-key") }
func (r testRing) FirstHS() []byte        { return r.GetHSByKID("") }
func (r testRing) AllHS() [][]byte        { return [][]byte{r.FirstHS()} }

// 经真实 gRPC 编解码及生产 JWT 拦截器证明：Admin、缺失/伪造/过期凭据
// 不能以请求 tenant/plugin/subscriber 冒充服务身份。拒绝发生在任何 DB IO 前。
func TestServiceRPCRejectsUnauthenticatedAndAdminCredentials(t *testing.T) {
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer(grpc.UnaryInterceptor(middleware.UnaryAuth(middleware.JWTVerifyConfig{Ring: testRing{}})), grpc.StreamInterceptor(middleware.StreamAuth(middleware.JWTVerifyConfig{Ring: testRing{}})))
	svc := channel.NewService(&gorm.DB{})
	pb.RegisterEventDeliveryServiceServer(server, NewEventDeliveryServer(svc))
	pb.RegisterEventSubscriberServiceServer(server, NewEventSubscriberServer(svc, defaultPollInterval))
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close(); <-done })
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := pb.NewEventDeliveryServiceClient(conn)
	const tenant = "6b5d0240-9920-46da-b707-88200e0f51ea"
	request := &pb.PublishEventRequest{ContractVersion: channel.Contract, PluginId: "com.powerx.plugins.scrm", TenantUuid: tenant, EventId: "same", IdempotencyKey: "same", Topic: "irrelevant", Version: "v1", Payload: []byte(`{}`)}
	admin := &reqctx.CoreXClaims{TenantUUID: tenant, UserID: 1, IsRoot: true, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx", Subject: "root", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	validAdmin, err := jwt.NewWithClaims(jwt.SigningMethodHS256, admin).SignedString(testRing{}.FirstHS())
	require.NoError(t, err)
	for _, token := range []string{"", "Bearer forged", "Bearer " + validAdmin} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", token))
		_, err = client.PublishEvent(ctx, request)
		require.Equal(t, codes.Unauthenticated, status.Code(err))
		_, err = client.AckDelivery(ctx, &pb.AckDeliveryRequest{ContractVersion: channel.Contract, TenantUuid: tenant, PluginId: request.PluginId, SubscriberId: request.PluginId, DeliveryId: "d0a2df08-2246-4bd5-b629-e3fa5faed0d5"})
		require.Equal(t, codes.Unauthenticated, status.Code(err))
		stream, e := pb.NewEventSubscriberServiceClient(conn).Subscribe(ctx, &pb.SubscribeRequest{ContractVersion: channel.Contract, TenantUuid: tenant, PluginId: request.PluginId, SubscriberId: request.PluginId, Topics: []string{"irrelevant"}, BatchSize: 1, SupportedVersions: []string{"v1"}, CompatibilityMode: pb.VersionCompatibilityMode_VERSION_COMPATIBILITY_MODE_STRICT})
		require.NoError(t, e)
		_, e = stream.Recv()
		require.Equal(t, codes.Unauthenticated, status.Code(e))
		cancel()
	}
	request.ContractVersion = ""
	_, err = client.PublishEvent(context.Background(), request)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Contains(t, status.Convert(err).Message(), "CONTRACT_REQUIRED")
}

func TestTenantHelperRejectsRequestOverride(t *testing.T) {
	const tenant = "6b5d0240-9920-46da-b707-88200e0f51ea"
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant})
	_, err := tenantUUIDFromRequest(ctx, "a5c18be4-2c3b-4852-80bc-ef5d330e89af")
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = tenantUUIDFromRequest(context.Background(), tenant)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	service := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts"}})
	_, err = tenantUUIDFromRequest(service, tenant)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}
