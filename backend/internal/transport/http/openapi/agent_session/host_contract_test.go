package agent_session

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_run"
	service "github.com/ArtisanCloud/PowerX/internal/service/agent_session"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type contractExecutor struct{ fail bool }

func (e contractExecutor) Execute(context.Context, service.Session, service.Message) (string, error) {
	if e.fail {
		return "", errors.New("test_dependency_failure")
	}
	return "contract-output", nil
}

func TestSignedSTSHTTPContract(t *testing.T) {
	old := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = old })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&m.ServiceSession{}, &m.ServiceMessage{}, &m.ServiceInvocation{}, &capmodel.CapabilityRecord{}, &capmodel.CapabilityRegistration{}, &setting.PluginInstanceConfig{}))
	require.NoError(t, db.Exec(`CREATE TABLE agents (id integer primary key, uuid text, tenant_uuid text, owner_plugin_id text, status text, deleted_at datetime)`).Error)
	tenant, otherTenant, agent := uuid.NewString(), uuid.NewString(), uuid.NewString()
	require.NoError(t, db.Exec("INSERT INTO agents (uuid,tenant_uuid,owner_plugin_id,status) VALUES (?,?,?,?)", agent, tenant, "plugin.test", "active").Error)
	for _, capability := range []string{service.SessionCapability, service.InvokeCapability} {
		require.NoError(t, db.Create(&capmodel.CapabilityRecord{CapabilityID: capability, PluginID: "core", PluginVersion: "1", Title: "test", CapabilitiesHash: "hash", ProtocolHash: "hash", Status: "published"}).Error)
		for _, scope := range []string{tenant, otherTenant} {
			require.NoError(t, db.Create(&capmodel.CapabilityRegistration{TenantUUID: scope, CapabilityID: capability, Status: "published", ContractRef: "test", Version: 1}).Error)
		}
	}
	for _, scope := range []string{tenant, otherTenant} {
		for _, plugin := range []string{"plugin.test", "plugin.other"} {
			raw, _ := json.Marshal(map[string]any{"client_id": plugin, "allowed_capabilities": []string{service.SessionCapability, service.InvokeCapability}})
			require.NoError(t, db.Create(&setting.PluginInstanceConfig{TenantUUID: scope, PluginID: plugin, Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(raw)}).Error)
		}
	}
	secret := []byte("session-contract-test-key-not-runtime-secret")
	sign := func(scope, plugin, issuer, audience string) string {
		claims := reqctx.CoreXClaims{TenantUUID: scope, PluginID: plugin, RegisteredClaims: jwt.RegisteredClaims{Issuer: issuer, Subject: "client:" + plugin, Audience: jwt.ClaimStrings{audience}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)), IssuedAt: jwt.NewNumericDate(time.Now())}}
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
		require.NoError(t, err)
		return token
	}
	token := sign(tenant, "plugin.test", "powerx-sts", "powerx:api")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := NewHandler(service.NewServiceWithExecutor(db, contractExecutor{}))
	h.Register(router.Group("/api/v1", middleware.JwtMiddleware(secret, "powerx-sts", []string{"powerx:api"}, nil, nil)))
	const base = "/api/v1/tenant/agent/sessions"
	request := func(method, path, body, auth, key string, headers map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if auth != "" {
			r.Header.Set("Authorization", "Bearer "+auth)
		}
		if key != "" {
			r.Header.Set("Idempotency-Key", key)
		}
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	for _, bad := range []string{"", "malformed", sign(tenant, "plugin.test", "wrong", "powerx:api"), sign(tenant, "plugin.test", "powerx-sts", "user")} {
		w := request("GET", base, "", bad, "", nil)
		require.Equal(t, 401, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), `"reason_code":"AGENT_SESSION_UNAUTHORIZED"`)
	}
	w := request("POST", base, `{"agent_uuid":"`+agent+`"}`, token, "", nil)
	require.Equal(t, 201, w.Code, w.Body.String())
	var created struct {
		Data service.Session `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	path := base + "/" + created.Data.SessionUUID.String()
	for _, bad := range []struct {
		path, body string
		headers    map[string]string
	}{
		{base, `{"agent_uuid":"` + agent + `","tenant_uuid":"` + otherTenant + `"}`, nil},
		{base, `{"agentId":1}`, nil}, {base, `{"agent_uuid":"1"}`, nil},
		{base, `{"agent_uuid":"` + agent + `","agent_uuid":"` + agent + `"}`, nil},
		{base, `{"Agent_UUID":"` + agent + `"}`, nil},
		{base, `null`, nil},
		{base + "?tenant_uuid=" + otherTenant, `{"agent_uuid":"` + agent + `"}`, nil},
		{base, `{"agent_uuid":"` + agent + `"}`, map[string]string{"X-Tenant-UUID": otherTenant}},
	} {
		w = request("POST", bad.path, bad.body, token, "", bad.headers)
		require.Equal(t, 400, w.Code, w.Body.String())
	}
	for _, other := range []string{sign(tenant, "plugin.other", "powerx-sts", "powerx:api"), sign(otherTenant, "plugin.test", "powerx-sts", "powerx:api")} {
		w = request("GET", path, "", other, "", nil)
		require.Equal(t, 404, w.Code, w.Body.String())
	}
	w = request("POST", path+"/messages", `{"role":"system","content":"test"}`, token, "m", nil)
	require.Equal(t, 400, w.Code)
	w = request("POST", path+"/messages", `{"role":"user","content":"test"}`, token, "m", nil)
	require.Equal(t, 201, w.Code, w.Body.String())
	var message struct {
		Data service.Message `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &message))
	w = request("POST", path+"/messages", `{"role":"user","content":"different"}`, token, "m", nil)
	require.Equal(t, 409, w.Code)
	w = request("POST", path+"/invocations", `{"message_uuid":"`+message.Data.MessageUUID.String()+`"}`, token, "run", nil)
	require.Equal(t, 202, w.Code, w.Body.String())
	var invocation struct {
		Data service.Invocation `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &invocation))
	runPath := path + "/invocations/" + invocation.Data.InvocationUUID.String()
	require.Eventually(t, func() bool {
		w = request("GET", runPath, "", token, "", nil)
		return strings.Contains(w.Body.String(), `"status":"succeeded"`)
	}, 3*time.Second, 20*time.Millisecond)
	w = request("GET", runPath+"/events", "", token, "", nil)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "id: 1\nevent: state")
	require.Contains(t, w.Body.String(), "id: 2\nevent: final")
	require.Contains(t, w.Body.String(), "id: 3\nevent: end")
	// Cursor 1 acknowledges state, so a reconnect only replays terminal frames.
	w = request("GET", runPath+"/events", "", token, "", map[string]string{"Last-Event-ID": "1"})
	require.Equal(t, 200, w.Code)
	require.NotContains(t, w.Body.String(), "id: 1\nevent: state")
	require.Contains(t, w.Body.String(), "id: 2\nevent: final")
	require.Contains(t, w.Body.String(), "id: 3\nevent: end")
	w = request("GET", runPath+"/events", "", token, "", map[string]string{"Last-Event-ID": "4"})
	require.Equal(t, 409, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"reason_code":"AGENT_SESSION_EVENT_CURSOR_EXPIRED"`)
	// A durable reader uses the same authenticated route, with unbounded
	// Redis event_seq cursors and no call to the execution method.
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, err := agent_run.NewRedisStore(client)
	require.NoError(t, err)
	snapshot := agent_run.Snapshot{TenantUUID: tenant, Env: "dev", RunID: invocation.Data.InvocationUUID.String(),
		SessionID: created.Data.SessionUUID.String(), MessageID: message.Data.MessageUUID.String(),
		TraceID: invocation.Data.TraceUUID.String(), Status: "accepted", DeadlineAt: invocation.Data.DeadlineAt}
	current, err := store.Create(context.Background(), snapshot)
	require.NoError(t, err)
	for i := 0; i < 4; i++ {
		status := "running"
		if i == 3 {
			status = "completed"
		}
		current, err = store.Transition(context.Background(), snapshot, current.Version, status, "agent_run.task_status")
		require.NoError(t, err)
	}
	durable := service.NewService(db)
	require.NoError(t, durable.ConfigureRunEvents(store, "dev"))
	h.svc = durable
	w = request("GET", runPath+"/events", "", token, "", map[string]string{"Last-Event-ID": "3"})
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), "id: 3\n")
	require.Contains(t, w.Body.String(), "id: 4\nevent: agent_run.task_status")
	require.Contains(t, w.Body.String(), "id: 5\nevent: agent_run.task_status")
	w = request("GET", runPath+"/events", "", token, "", map[string]string{"Last-Event-ID": "6"})
	require.Equal(t, 409, w.Code, w.Body.String())
	eventKey := "agent:run:{" + tenant + ":dev:" + invocation.Data.InvocationUUID.String() + "}:events"
	require.NoError(t, client.XDel(context.Background(), eventKey, "1-0").Err())
	w = request("GET", runPath+"/events", "", token, "", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "id: 5\nevent: agent_run.snapshot")
	require.Contains(t, w.Body.String(), `"tasks":[]`)
	// Existing signed tokens lose access immediately when persisted grants change.
	require.NoError(t, db.Model(&setting.PluginInstanceConfig{}).Where("tenant_uuid = ? AND plugin_id = ?", tenant, "plugin.test").Update("value_json", datatypes.JSON([]byte(`{"client_id":"plugin.test","allowed_capabilities":[]}`))).Error)
	w = request("GET", path, "", token, "", nil)
	require.Equal(t, 403, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"reason_code":"AGENT_SESSION_FORBIDDEN"`)
	// A dependency outage must not masquerade as a missing session.
	require.NoError(t, sqlDB.Close())
	w = request("GET", path, "", token, "", nil)
	require.Equal(t, 503, w.Code, w.Body.String())
}
