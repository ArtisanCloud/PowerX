package capability_registry

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
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
	result, err := svc.invokeREST(context.Background(), "com.corex.biz.media.list", payload, "trace-http", false)
	require.NoError(t, err)
	require.Equal(t, "ok", result["status"])
}

func TestInvocationServiceInvokeRESTAppliesPathParamsAndTenantHeader(t *testing.T) {
	t.Parallel()
	testutil.SkipIfNoLocalListener(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/media/assets/asset-001/presign", r.URL.Path)
		require.Equal(t, "tenant-001", r.Header.Get("X-Tenant-UUID"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"https://assets.example/signed","method":"PUT"}`))
	}))
	defer server.Close()

	svc := &InvocationService{
		httpClient:  server.Client(),
		httpBaseURL: server.URL,
	}

	ctx := context.WithValue(context.Background(), "tenant_uuid", "tenant-001")
	payload := restInvokePayload{
		Method:   http.MethodPost,
		Endpoint: "/media/assets/{uuid}/presign",
		Body: map[string]interface{}{
			"uuid":   "asset-001",
			"action": "upload",
		},
	}
	result, err := svc.invokeREST(ctx, "com.corex.media.assets.manage", payload, "trace-http", false)
	require.NoError(t, err)
	require.Equal(t, "https://assets.example/signed", result["url"])
}

func TestBuildRESTInvokePayloadRequiresMethod(t *testing.T) {
	t.Parallel()

	_, err := buildRESTInvokePayloadWithDefaults(map[string]interface{}{
		"endpoint": "/api/v1/templates",
	}, "", nil)
	require.ErrorContains(t, err, "REST invocation requires method")
}

func TestWrapPluginCapabilityInvokePayloadAddsEnvelope(t *testing.T) {
	t.Parallel()

	raw := map[string]interface{}{
		"action": "create",
	}
	wrapped := wrapPluginCapabilityInvokePayload(InvocationInput{
		CapabilityID:      "com.powerx.plugins.base.local.template.prepare",
		PreferredProtocol: "rest",
		Payload:           raw,
		Context: map[string]interface{}{
			"trace_id": "trace-prepare",
		},
	}, "/_p/com.powerx.plugins.base.local/api/v1/integration/capabilities/invoke", map[string]string{"method": "POST"})

	restPayload, err := buildRESTInvokePayloadWithDefaults(wrapped, "/_p/com.powerx.plugins.base.local/api/v1/integration/capabilities/invoke", map[string]string{"method": "POST"})
	require.NoError(t, err)
	require.Equal(t, http.MethodPost, restPayload.Method)
	require.Equal(t, "/_p/com.powerx.plugins.base.local/api/v1/integration/capabilities/invoke", restPayload.Endpoint)

	body, ok := restPayload.Body.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "com.powerx.plugins.base.local.template.prepare", body["capabilityId"])
	require.Equal(t, "create", body["action"])
	require.Equal(t, "rest", body["preferredProtocol"])

	payload, ok := body["payload"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "create", payload["action"])

	metadata, ok := body["metadata"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "com.powerx.plugins.base.local.template.prepare", metadata["capability_id"])
	require.Equal(t, "trace-prepare", metadata["trace_id"])
}

func TestInvocationServiceInvokeRESTForwardsAuthorizationFromContext(t *testing.T) {
	t.Parallel()
	testutil.SkipIfNoLocalListener(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer inherited", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	svc := &InvocationService{
		httpClient:  server.Client(),
		httpBaseURL: server.URL,
	}

	ctx := context.WithValue(context.Background(), "authorization", "Bearer inherited")
	result, err := svc.invokeREST(ctx, "com.powerx.plugins.base.local.template.prepare", restInvokePayload{
		Method:   http.MethodPost,
		Endpoint: "/_p/com.powerx.plugins.base.local/api/v1/integration/capabilities/invoke",
		Body:     map[string]interface{}{"action": "create"},
	}, "trace-http", false)
	require.NoError(t, err)
	require.Equal(t, "ok", result["status"])
}

func TestInvocationServiceInvokeCoreCapabilityBeforeRESTProxy(t *testing.T) {
	t.Parallel()

	invoker := &stubCoreInvoker{
		result: map[string]interface{}{"items": []interface{}{"customer-1"}},
	}
	svc := &InvocationService{coreInvoker: invoker}

	result, handled, err := svc.invokeCoreCapability(context.Background(), InvocationInput{
		CapabilityID: "com.corex.customer.accounts.admin_manage",
		TenantUUID:   "11111111-1111-1111-1111-111111111111",
		Payload: map[string]interface{}{
			"method":   http.MethodGet,
			"endpoint": "/api/v1/admin/customers/accounts",
			"query": map[string]interface{}{
				"page": "1",
			},
		},
	}, routerInvokeResultForTest("/api/v1/admin/customers/accounts", http.MethodGet))

	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, invoker.result, result)
	require.Equal(t, "com.corex.customer.accounts.admin_manage", invoker.input.CapabilityID)
	require.Equal(t, "GET", invoker.input.Method)
	require.Equal(t, "/api/v1/admin/customers/accounts", invoker.input.Endpoint)
	require.Equal(t, "1", invoker.input.Query["page"])
}

func TestInvocationServiceInvokeCoreCapabilityNotHandledFallsBack(t *testing.T) {
	t.Parallel()

	svc := &InvocationService{coreInvoker: &stubCoreInvoker{err: ErrCoreCapabilityNotHandled}}
	result, handled, err := svc.invokeCoreCapability(context.Background(), InvocationInput{
		CapabilityID: "com.corex.media.assets.manage",
		TenantUUID:   "11111111-1111-1111-1111-111111111111",
		Payload: map[string]interface{}{
			"method":   http.MethodGet,
			"endpoint": "/api/v1/admin/media/assets",
		},
	}, routerInvokeResultForTest("/api/v1/admin/media/assets", http.MethodGet))

	require.NoError(t, err)
	require.False(t, handled)
	require.Nil(t, result)
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

type stubCoreInvoker struct {
	input  CoreCapabilityInvokeInput
	result map[string]interface{}
	err    error
}

func (s *stubCoreInvoker) InvokeCoreCapability(_ context.Context, in CoreCapabilityInvokeInput) (map[string]interface{}, error) {
	s.input = in
	if s.err != nil {
		return nil, s.err
	}
	if s.result == nil {
		return nil, errors.New("stub result missing")
	}
	return s.result, nil
}

func routerInvokeResultForTest(endpoint string, method string) router.InvokeResult {
	return router.InvokeResult{
		Transport: "rest",
		Endpoint:  endpoint,
		Labels: map[string]string{
			"method": method,
		},
	}
}

func TestShouldUseAIMultimodalTimeoutByLabels(t *testing.T) {
	t.Parallel()

	ok := shouldUseAIMultimodalTimeout(
		"com.plugin.vendor.chat.invoke",
		"/v1/chat/completions",
		map[string]string{"tool_scope": "ai.llm"},
		map[string]interface{}{},
	)
	require.True(t, ok)
}

func TestShouldUseAIMultimodalTimeoutByPayload(t *testing.T) {
	t.Parallel()

	ok := shouldUseAIMultimodalTimeout(
		"com.plugin.vendor.chat.invoke",
		"/v1/chat/completions",
		nil,
		map[string]interface{}{
			"body": map[string]interface{}{
				"model_key": "openai:gpt-4o",
				"messages":  []interface{}{},
			},
		},
	)
	require.True(t, ok)
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
