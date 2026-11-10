package capability_registry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	caprouter "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/pluginpermission"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type streamModelVerifier struct {
	err   error
	calls int
}

func (m *streamModelVerifier) VerifyModelKey(_ context.Context, tenant, env, modality, key string) error {
	m.calls++
	if tenant != apiKeyCatalogTestTenant || env != "dev" || modality != "llm" || key != "openai/fixture" {
		return errors.New("unexpected model binding")
	}
	return m.err
}

type streamVersionLock struct{ err error }

func (v *streamVersionLock) Enforce(context.Context, string, string, string) error { return v.err }
func (v *streamVersionLock) IsUpgradeError(err error) bool {
	return errors.Is(err, cap.ErrManualUpgradeRequired)
}

type streamAuthorizer struct {
	err   error
	calls int
}

func (a *streamAuthorizer) AuthorizeInvocation(context.Context, cap.CapabilityInvokeRequest) error {
	a.calls++
	return a.err
}

type httpTestTicketIssuer struct{ calls int }

func (i *httpTestTicketIssuer) IssueServiceInvocation(context.Context, cap.ServiceInvocationBinding) (string, error) {
	i.calls++
	return "controlled-service-ticket", nil
}
func (i *httpTestTicketIssuer) DiscardServiceInvocation(context.Context, string) {}

type invocationHTTPFixture struct {
	db                             *gorm.DB
	engine                         *gin.Engine
	svc                            *cap.InvocationService
	key                            gw.IntegrationGatewayAPIKey
	capID, owner, endpoint, secret string
	calls                          atomic.Int32
	responseStatus                 int
	responseBody                   string
	contentType                    string
	model                          *streamModelVerifier
	version                        *streamVersionLock
	authz                          *streamAuthorizer
	core                           *fixedCoreStreamFixture
	issuer                         *httpTestTicketIssuer
	target                         *httptest.Server
	runtime                        pm.Plugin
	lookupErr                      error
}

func newInvocationHTTPFixture(t *testing.T, plugin bool, coreInvokers ...cap.CoreCapabilityInvoker) *invocationHTTPFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&cm.CapabilityRecord{}, &cm.CapabilityRegistration{}, &cm.RoutingPolicy{}, &cm.AdapterEndpoint{}, &cm.FallbackPlan{}, &im.APIKey{}, &im.APIKeyProfile{}, &im.Permission{}, &gw.IntegrationGatewayAPIKey{}, &gw.IntegrationGatewayAPIKeyPermission{}))
	f := &invocationHTTPFixture{db: db, capID: "com.corex.ai.llm.stream", owner: "corex.platform", endpoint: "/api/v1/ai/llm/stream", secret: "pxk_synthetic_http_fixture_" + t.Name(), responseStatus: 200, contentType: "text/event-stream", responseBody: "event: done\ndata: {\"text\":\"hello\"}\n\n", model: &streamModelVerifier{}, version: &streamVersionLock{}, authz: &streamAuthorizer{}, issuer: &httpTestTicketIssuer{}}
	if plugin {
		f.capID = "com.example.scrm.messages.reply"
		f.owner = "com.example.scrm"
		f.endpoint = "/api/v1/service/messages/reply"
		f.contentType = "application/json"
		f.responseBody = `{"accepted":true}`
	}
	profile := im.APIKeyProfile{TenantUUID: apiKeyCatalogTestTenant, Key: "http-fixture", Name: "Fixture", Status: 1}
	require.NoError(t, db.Create(&profile).Error)
	sum := sha256.Sum256([]byte(f.secret))
	hash := hex.EncodeToString(sum[:])
	require.NoError(t, db.Create(&im.APIKey{TenantUUID: apiKeyCatalogTestTenant, ProfileID: profile.ID, KeyHash: hash}).Error)
	f.key = gw.IntegrationGatewayAPIKey{TenantUUID: apiKeyCatalogTestTenant, ProfileID: profile.ID, Name: "Fixture", KeyHash: hash, Status: "active"}
	require.NoError(t, db.Create(&f.key).Error)
	scope := "_scope.fixture.service"
	seedAPIKeyCatalogCapability(t, db, f.key, f.capID, scope)
	binding := cm.ProtocolBinding{Channel: "rest", Method: "POST", Endpoint: f.endpoint, AuthType: "tenant_jwt", ActorContext: "service_actor", ResourceScope: "tenant", SchemaRef: "fixture.yaml#/Request"}
	if plugin {
		binding.AuthType = cap.PluginServiceAuthType
		spec := pluginpermission.APIKeySpec{CapabilityID: f.capID, PluginID: f.owner, Scope: scope, Action: "read", ResourceType: "capability", ResourcePattern: f.capID, Effect: "allow"}
		meta, err := pluginpermission.ExplicitAPIKeyMetadata(&spec, f.owner, f.capID, true)
		require.NoError(t, err)
		raw, err := json.Marshal(meta)
		require.NoError(t, err)
		require.NoError(t, db.Model(&im.Permission{}).Where("resource=?", f.capID).Updates(map[string]interface{}{"meta": datatypes.JSON(raw), "source": "plugin:" + f.owner}).Error)
		require.NoError(t, db.Model(&gw.IntegrationGatewayAPIKeyPermission{}).Where("api_key_uuid=?", f.key.UUID).Updates(map[string]interface{}{"plugin_id": f.owner, "resource_type": "capability"}).Error)
	}
	protocols, err := json.Marshal([]cm.ProtocolBinding{binding})
	require.NoError(t, err)
	require.NoError(t, db.Model(&cm.CapabilityRecord{}).Where("capability_id=?", f.capID).Updates(map[string]interface{}{"plugin_id": f.owner, "plugin_version": "1.2.3", "protocols": datatypes.JSON(protocols)}).Error)
	policy := cm.RoutingPolicy{Strategy: "weighted_round_robin"}
	require.NoError(t, db.Create(&policy).Error)
	var registration cm.CapabilityRegistration
	require.NoError(t, db.Where("capability_id=?", f.capID).First(&registration).Error)
	require.NoError(t, db.Model(&registration).Update("routing_policy_id", policy.UUID).Error)
	endpoint := f.endpoint
	if plugin {
		endpoint = pm.CatalogPluginEndpoint(f.owner, "rest", endpoint)
	}
	require.NoError(t, db.Create(&cm.AdapterEndpoint{RegistrationID: registration.ID, CapabilityID: f.capID, TenantUUID: apiKeyCatalogTestTenant, AdapterID: "fixture", TransportType: "rest", Endpoint: endpoint, IsActive: true, Weight: 100, Labels: datatypes.JSON(`{"method":"POST"}`)}).Error)
	f.target = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		if r.Method != "POST" || r.URL.Path != f.endpoint {
			t.Errorf("routing changed: %s %s", r.Method, r.URL.Path)
		}
		if plugin {
			if r.Header.Get("Authorization") != "" || r.Header.Get(cap.PluginServiceIdentityHeader) != "controlled-service-ticket" {
				t.Error("service identity boundary failed")
			}
		} else {
			if r.Header.Get("Authorization") != "ApiKey "+f.secret || r.URL.Query().Get("env") != "dev" {
				t.Error("Core stream credentials/env lost")
			}
		}
		if r.Header.Get("X-PowerX-Is-Root") != "" {
			t.Error("forged header forwarded")
		}
		w.Header().Set("Content-Type", f.contentType)
		w.WriteHeader(f.responseStatus)
		_, _ = w.Write([]byte(f.responseBody))
	}))
	t.Cleanup(f.target.Close)
	f.core = &fixedCoreStreamFixture{}
	catalog := cap.NewRegistryService(cap.RegistryServiceOptions{DB: db})
	coreInvoker := cap.CoreCapabilityInvoker(f.core)
	if len(coreInvokers) > 0 {
		coreInvoker = coreInvokers[0]
	}
	f.svc = cap.NewInvocationService(cap.InvocationServiceOptions{CoreInvoker: coreInvoker, Catalog: catalog, Router: caprouter.NewService(caprouter.ServiceOptions{DB: db}), HTTPBaseURL: f.target.URL, HTTPClient: f.target.Client(), AIModalHTTPClient: f.target.Client(), ModelVerifier: f.model, VersionLock: f.version, ServiceIdentityIssuer: f.issuer})
	f.runtime = pm.Plugin{Version: "1.2.3", State: pm.StateEnabled}
	f.svc.BindPluginServiceTargetResolver(func(ctx context.Context, tenant, pluginID, version, path string) (string, error) {
		if err := cap.ValidatePluginServiceRuntime(f.runtime, version, f.lookupErr); err != nil {
			return "", err
		}
		return f.target.URL + path, nil
	})
	selector := cap.NewSelector(cap.SelectorOptions{Invoker: f.svc, Authorizer: f.authz})
	f.engine = gin.New()
	f.engine.Use(middleware.APIKeyOrJwtMiddleware(db, []byte("synthetic-test-jwt-secret"), "powerx", nil, nil, nil))
	RegisterTenantRoutes(f.engine.Group("/api/v1"), &shared.Deps{DB: db, CapabilityCatalogSvc: catalog, CapabilityInvocationSvc: f.svc, CapabilitySelector: selector})
	return f
}
func (f *invocationHTTPFixture) request(t *testing.T, path string, envelope map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(envelope)
	require.NoError(t, err)
	req := httptest.NewRequest("POST", path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "ApiKey "+f.secret)
	// These headers must never determine the Core destination.
	req.Host = "attacker.invalid"
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	return w
}
func streamEnvelope(f *invocationHTTPFixture) map[string]interface{} {
	return map[string]interface{}{"capability_id": f.capID, "payload": map[string]interface{}{"body": map[string]interface{}{"model_key": "openai/fixture", "inputs": []interface{}{map[string]interface{}{"type": "text", "text": "hi"}}}}}
}

func TestInvokeCapabilityStreamAuthorizedFixedCoreSSE(t *testing.T) {
	f := newInvocationHTTPFixture(t, false)
	envelope := streamEnvelope(f)
	payload := envelope["payload"].(map[string]interface{})
	payload["method"] = "POST"
	payload["endpoint"] = f.endpoint
	payload["headers"] = map[string]interface{}{"Content-Type": "application/json"}
	w := f.request(t, "/api/v1/tenant/invocations/stream?env=dev", envelope)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "event: done")
	require.NotEmpty(t, w.Header().Get("X-Trace-ID"))
	require.EqualValues(t, 1, f.calls.Load())
	require.Equal(t, 1, f.model.calls)
	require.Equal(t, 1, f.authz.calls)
}

func TestInvokeCapabilityStreamRejectsUnauthorizedOrUnboundRequests(t *testing.T) {
	tests := []struct {
		name   string
		alter  func(*testing.T, *invocationHTTPFixture, map[string]interface{})
		status int
	}{
		{"arbitrary URL", func(t *testing.T, f *invocationHTTPFixture, e map[string]interface{}) {
			e["payload"].(map[string]interface{})["endpoint"] = "http://attacker.invalid/arbitrary"
		}, 400},
		{"method override", func(t *testing.T, f *invocationHTTPFixture, e map[string]interface{}) {
			e["payload"].(map[string]interface{})["method"] = "DELETE"
		}, 400},
		{"credential override", func(t *testing.T, f *invocationHTTPFixture, e map[string]interface{}) {
			e["payload"].(map[string]interface{})["headers"] = map[string]interface{}{"Authorization": "Bearer forged"}
		}, 400},
		{"context injection", func(t *testing.T, f *invocationHTTPFixture, e map[string]interface{}) {
			e["context"] = map[string]interface{}{"is_root": true}
		}, 400},
		{"unpublished", func(t *testing.T, f *invocationHTTPFixture, e map[string]interface{}) {
			require.NoError(t, f.db.Model(&cm.CapabilityRecord{}).Where("capability_id=?", f.capID).Update("status", "draft").Error)
		}, 403},
		{"unknown capability", func(t *testing.T, f *invocationHTTPFixture, e map[string]interface{}) {
			e["capability_id"] = "arbitrary.unpublished.stream"
		}, 403},
		{"registration removed", func(t *testing.T, f *invocationHTTPFixture, e map[string]interface{}) {
			require.NoError(t, f.db.Model(&cm.CapabilityRegistration{}).Where("capability_id=?", f.capID).Update("status", "disabled").Error)
		}, 403},
		{"latest registration disabled", func(t *testing.T, f *invocationHTTPFixture, e map[string]interface{}) {
			require.NoError(t, f.db.Create(&cm.CapabilityRegistration{CapabilityID: f.capID, TenantUUID: apiKeyCatalogTestTenant, Version: 2, Status: "disabled", RoutingPolicyID: uuid.New(), ContractRef: "test"}).Error)
		}, 403},
		{"grant removed", func(t *testing.T, f *invocationHTTPFixture, e map[string]interface{}) {
			require.NoError(t, f.db.Where("api_key_uuid=?", f.key.UUID).Delete(&gw.IntegrationGatewayAPIKeyPermission{}).Error)
		}, 403},
		{"revoked credential", func(t *testing.T, f *invocationHTTPFixture, e map[string]interface{}) {
			require.NoError(t, f.db.Model(&f.key).Update("status", "revoked").Error)
		}, 401},
		{"cross tenant registration", func(t *testing.T, f *invocationHTTPFixture, e map[string]interface{}) {
			require.NoError(t, f.db.Model(&cm.CapabilityRegistration{}).Where("capability_id=?", f.capID).Update("tenant_uuid", uuid.NewString()).Error)
		}, 403},
		{"model access denied", func(t *testing.T, f *invocationHTTPFixture, e map[string]interface{}) {
			f.model.err = dto.NewErrorWithCode(403, "AI_MODEL_KEY_FORBIDDEN", "Model denied", nil)
		}, 403},
		{"version locked", func(t *testing.T, f *invocationHTTPFixture, e map[string]interface{}) {
			f.version.err = cap.ErrManualUpgradeRequired
		}, 424},
		{"selector denies", func(t *testing.T, f *invocationHTTPFixture, e map[string]interface{}) {
			f.authz.err = cap.ErrSelectorSafeModeActive
		}, 503},
		{"adapter drift", func(t *testing.T, f *invocationHTTPFixture, e map[string]interface{}) {
			require.NoError(t, f.db.Model(&cm.AdapterEndpoint{}).Where("capability_id=?", f.capID).Update("endpoint", "http://attacker.invalid").Error)
		}, 400},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newInvocationHTTPFixture(t, false)
			e := streamEnvelope(f)
			test.alter(t, f, e)
			w := f.request(t, "/api/v1/tenant/invocations/stream?env=dev", e)
			require.Equal(t, test.status, w.Code, w.Body.String())
			require.Zero(t, f.calls.Load())
			require.Zero(t, f.issuer.calls)
		})
	}
}

func TestInvokeCapabilityStreamRejectsTicketPluginAndUnsafeResponses(t *testing.T) {
	t.Run("plugin stream unsupported", func(t *testing.T) {
		f := newInvocationHTTPFixture(t, true)
		w := f.request(t, "/api/v1/tenant/invocations/stream", map[string]interface{}{"capability_id": f.capID, "payload": map[string]interface{}{"body": map[string]interface{}{}}})
		require.Equal(t, 400, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "CAPABILITY_SERVICE_STREAM_UNSUPPORTED")
		require.Zero(t, f.calls.Load())
		require.Zero(t, f.issuer.calls)
	})
	for _, status := range []int{200, 302, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := newInvocationHTTPFixture(t, false)
			f.contentType = "application/json"
			f.responseStatus = status
			f.responseBody = `{"error":"private-provider-secret"}`
			w := f.request(t, "/api/v1/tenant/invocations/stream?env=dev", streamEnvelope(f))
			require.NotEqual(t, 200, w.Code)
			require.NotContains(t, w.Body.String(), "private-provider-secret")
			require.EqualValues(t, 1, f.calls.Load())
		})
	}
}

type fixedCoreStreamFixture struct {
	calls int
	input cap.CoreCapabilityInvokeInput
}

func (f *fixedCoreStreamFixture) InvokeCoreCapability(context.Context, cap.CoreCapabilityInvokeInput) (map[string]any, error) {
	return nil, cap.ErrCoreCapabilityNotHandled
}
func (f *fixedCoreStreamFixture) InvokeCoreCapabilityStream(_ context.Context, in cap.CoreCapabilityInvokeInput) (io.ReadCloser, error) {
	f.calls++
	f.input = in
	return io.NopCloser(strings.NewReader("event: ready\ndata: {}\n\n")), nil
}
func TestInvokeCapabilityStreamDeclaredCoreInternalKeepsAuthorizationAndFixedRoute(t *testing.T) {
	f := newInvocationHTTPFixture(t, false)
	binding, _ := json.Marshal([]cm.ProtocolBinding{{Channel: "core_internal", Method: "INVOKE", Endpoint: "core://runtime/realtime", AuthType: "api_key_or_sts", ActorContext: "service_actor", ResourceScope: "tenant"}})
	require.NoError(t, f.db.Model(&cm.CapabilityRecord{}).Where("capability_id=?", f.capID).Update("protocols", datatypes.JSON(binding)).Error)
	require.NoError(t, f.db.Model(&cm.AdapterEndpoint{}).Where("capability_id=?", f.capID).Updates(map[string]any{"transport_type": "core_internal", "endpoint": "core://runtime/realtime", "labels": datatypes.JSON(`{"method":"INVOKE"}`)}).Error)
	envelope := streamEnvelope(f)
	envelope["preferred_protocol"] = "core_internal"
	response := f.request(t, "/api/v1/tenant/invocations/stream?env=dev", envelope)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "event: ready")
	require.Equal(t, 1, f.core.calls)
	require.Zero(t, f.calls.Load())
	require.Equal(t, "core://runtime/realtime", f.core.input.Endpoint)
	require.Equal(t, "INVOKE", f.core.input.Method)
	require.Equal(t, 1, f.authz.calls)
	require.NoError(t, f.db.Where("api_key_uuid=?", f.key.UUID).Delete(&gw.IntegrationGatewayAPIKeyPermission{}).Error)
	response = f.request(t, "/api/v1/tenant/invocations/stream?env=dev", envelope)
	require.Equal(t, 403, response.Code)
	require.Equal(t, 1, f.core.calls)
}
