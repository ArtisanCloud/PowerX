package integrationgatewaycontract

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	registryv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/registry/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	capregistrygrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/capability_registry"
	capregistryhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestCapabilityRegistryHTTPErrors(t *testing.T) {
	env := newCapabilityRegistryHTTPEnv(t)
	t.Cleanup(env.Close)

	t.Run("create capability validation error returns dto envelope", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/capability-registry/capabilities", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()

		env.Engine.ServeHTTP(resp, req)
		require.Equal(t, http.StatusUnprocessableEntity, resp.Code)

		var envelope errorEnvelope
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &envelope))
		require.Equal(t, http.StatusUnprocessableEntity, envelope.Code)
		require.Equal(t, "registry.invalid_payload", envelope.Message)
		require.NotZero(t, envelope.Timestamp)
		require.NotEmpty(t, envelope.Error)
	})

	tenantUUID := "fd8080ba-8bdd-46d3-9cf5-9e5eb4caa001"

	t.Run("get capability not found returns dto envelope", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/admin/capability-registry/capabilities/cap.missing/tenants/"+tenantUUID, nil)
		resp := httptest.NewRecorder()

		env.Engine.ServeHTTP(resp, req)
		require.Equal(t, http.StatusNotFound, resp.Code)

		var envelope errorEnvelope
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &envelope))
		require.Equal(t, http.StatusNotFound, envelope.Code)
		require.Equal(t, "registry.not_found", envelope.Message)
		require.NotEmpty(t, envelope.Error)
		require.Contains(t, envelope.Details, "code")
	})
}

func TestCapabilityRegistryGRPCErrors(t *testing.T) {
	env := newCapabilityRegistryWorkerEnv(t)
	t.Cleanup(env.Close)

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	capregistrygrpc.RegisterCapabilityRegistryServer(server, env.Service)

	done := make(chan struct{})
	go func() {
		_ = server.Serve(listener)
		close(done)
	}()
	t.Cleanup(func() {
		server.GracefulStop()
		_ = listener.Close()
		<-done
	})

	dialer := func(ctx context.Context, s string) (net.Conn, error) {
		return listener.Dial()
	}
	conn, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(dialer), grpc.WithInsecure())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := registryv1.NewCapabilityRegistryServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	t.Run("create capability missing registration returns invalid argument", func(t *testing.T) {
		_, err := client.CreateCapability(ctx, &registryv1.CreateCapabilityRequest{})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.InvalidArgument, st.Code())
	})

	t.Run("update capability version conflict returns failed precondition", func(t *testing.T) {
		registration := samplePBRegistration("cap.grpc.conflict", "91cc7f5a-1b3e-4f3c-b690-1bc4a1aab001")
		_, err := client.CreateCapability(ctx, &registryv1.CreateCapabilityRequest{
			Registration: registration,
		})
		require.NoError(t, err)

		stale := samplePBRegistration("cap.grpc.conflict", "91cc7f5a-1b3e-4f3c-b690-1bc4a1aab001")
		stale.Version = 99

		_, err = client.UpdateCapability(ctx, &registryv1.UpdateCapabilityRequest{
			Registration: stale,
		})
		require.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		require.Equal(t, codes.FailedPrecondition, st.Code())
	})
}

type errorEnvelope struct {
	Code      int                    `json:"code"`
	Message   string                 `json:"message"`
	Error     string                 `json:"error"`
	ErrorCode string                 `json:"error_code"`
	Details   map[string]interface{} `json:"details"`
	Timestamp int64                  `json:"timestamp"`
}

type capabilityRegistryHTTPEnv struct {
	*capabilityRegistryWorkerEnv
	Engine *gin.Engine
}

func newCapabilityRegistryHTTPEnv(t *testing.T) *capabilityRegistryHTTPEnv {
	base := newCapabilityRegistryWorkerEnv(t)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	protected := engine.Group("/api")

	capregistryhttp.RegisterAPIRoutes(nil, protected, &shared.Deps{
		CapabilityRegistrySvc: base.Service,
	})

	return &capabilityRegistryHTTPEnv{
		capabilityRegistryWorkerEnv: base,
		Engine:                      engine,
	}
}

func (e *capabilityRegistryHTTPEnv) Close() {
	e.capabilityRegistryWorkerEnv.Close()
}

func samplePBRegistration(capabilityID, tenantUUID string) *registryv1.CapabilityRegistration {
	return &registryv1.CapabilityRegistration{
		Id: &registryv1.TenantScopedId{
			CapabilityId: capabilityID,
			TenantUuid:   tenantUUID,
		},
		ContractRef: "contracts/exposure/mcp-tools.json",
		Status:      "published",
		Adapters: []*registryv1.AdapterEndpoint{
			{
				AdapterId:     "adapter-grpc",
				TransportType: "grpc",
				Endpoint:      "grpc://plugin.valid/Invoke",
				Weight:        100,
				TimeoutMs:     3000,
			},
		},
		RoutingPolicy: &registryv1.RoutingPolicy{
			Strategy:        "weighted_round_robin",
			CooldownSeconds: 60,
			RateLimit: &registryv1.RateLimit{
				Limit:         120,
				WindowSeconds: 60,
			},
		},
		ToolGrantIds: []string{"grant-grpc"},
	}
}
