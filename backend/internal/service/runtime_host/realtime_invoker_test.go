package runtime_host

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	fixture "github.com/ArtisanCloud/PowerX/internal/testutil/runtime_host"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func realtimeFixture(t *testing.T) (*cacheFixtureState, *RealtimeInvoker) {
	f := cacheFixture(t)
	f.db.Config.DisableForeignKeyConstraintWhenMigrating = true
	require.NoError(t, f.db.AutoMigrate(&cm.RoutingPolicy{}, &cm.AdapterEndpoint{}, &cm.FallbackPlan{}))
	for _, row := range []struct{ id, resource, action string }{{RealtimePublish, "service_publish", "publish"}, {RealtimeStream, "stream", "subscribe"}} {
		require.NoError(t, f.db.Create(&cm.CapabilityRecord{CapabilityID: row.id, PluginID: "corex.platform", PluginVersion: "1", Title: row.id, CapabilitiesHash: row.id, ProtocolHash: row.id, Status: "published"}).Error)
		require.NoError(t, f.db.Create(&cm.CapabilityRegistration{CapabilityID: row.id, TenantUUID: cacheTenant, ContractRef: RealtimeContract, Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
		meta, _ := json.Marshal(map[string]any{"capability_id": row.id, "api_key_explicit": true, "api_key": map[string]any{"scope": "_scope.runtime.realtime." + row.action, "action": row.action, "resource_type": "capability", "resource_pattern": "runtime_realtime_" + row.resource, "effect": "allow"}})
		require.NoError(t, f.db.Create(&im.Permission{Module: "runtime_realtime", Resource: row.resource, Action: row.action, Effect: "allow", Status: im.PermissionStatusActive, AllowAPIKey: true, Meta: datatypes.JSON(meta)}).Error)
		require.NoError(t, f.db.Create(&gm.IntegrationGatewayAPIKeyPermission{APIKeyUUID: f.key.UUID, Scope: "_scope.runtime.realtime." + row.action, Action: row.action, ResourceType: "capability", ResourcePattern: "runtime_realtime_" + row.resource, Effect: "allow"}).Error)
	}
	var credential setting.PluginInstanceConfig
	require.NoError(t, f.db.Where("tenant_uuid = ? AND plugin_id = ? AND key = ?", cacheTenant, "fixture-cache-plugin", "auth.credentials").First(&credential).Error)
	fixture.Grant(t, f.db, &credential, append(fixture.Capabilities, CacheServiceRead, CacheServiceManage, RealtimePublish, RealtimeStream)...)
	return f, NewRealtimeInvoker(f.db, NewService(f.db, f.redis, "fixture-realtime-env-"+uuid.NewString()))
}
func realtimeInput(operation string) cap.CoreCapabilityInvokeInput {
	body := map[string]any{"contract_version": RealtimeContract, "tenant_key": cacheTenant, "namespace": "fixture.realtime", "operation": operation}
	id := RealtimeStream
	if operation == "publish" {
		id = RealtimePublish
		body["event"] = RealtimeEvent{EventID: uuid.NewString(), TenantUUID: cacheTenant, Topic: "fixture.topic", TraceID: "fixture-trace", PayloadBase64: base64.StdEncoding.EncodeToString([]byte(`{"payload_version":1,"id":9007199254740993}`))}
	}
	if operation == "subscribe" {
		body["cursor"] = "0-0"
	}
	return cap.CoreCapabilityInvokeInput{CapabilityID: id, TenantUUID: cacheTenant, Method: "INVOKE", Endpoint: RealtimeEndpoint, Body: body, Payload: map[string]any{"body": body}}
}
func TestRealtimeTypedServiceIsolationAndStrictContract(t *testing.T) {
	f, invoker := realtimeFixture(t)
	ctx := context.Background()
	for _, identity := range []context.Context{f.api, f.sts} {
		out, err := invoker.InvokeCoreCapability(identity, realtimeInput("cursor"))
		require.NoError(t, err)
		require.Equal(t, "0-0", out["cursor"])
		out, err = invoker.InvokeCoreCapability(identity, realtimeInput("publish"))
		require.NoError(t, err)
		require.NotEqual(t, "0-0", out["cursor"])
	}
	other := NewRealtimeInvoker(f.db, NewService(f.db, f.redis, "another-environment"))
	out, err := other.InvokeCoreCapability(f.api, realtimeInput("cursor"))
	require.NoError(t, err)
	require.Equal(t, "0-0", out["cursor"])
	_, err = invoker.InvokeCoreCapability(ctx, realtimeInput("publish"))
	require.Error(t, err)
	for _, mutation := range []func(*cap.CoreCapabilityInvokeInput){func(in *cap.CoreCapabilityInvokeInput) { in.Endpoint = "https://fixture.invalid" }, func(in *cap.CoreCapabilityInvokeInput) { in.Method = "POST" }, func(in *cap.CoreCapabilityInvokeInput) { in.Body["tenant_key"] = uuid.NewString() }, func(in *cap.CoreCapabilityInvokeInput) {
		in.Body["headers"] = map[string]string{"Authorization": "fixture"}
	}, func(in *cap.CoreCapabilityInvokeInput) { in.Body["cursor"] = nil }, func(in *cap.CoreCapabilityInvokeInput) { in.Payload["headers"] = true }} {
		in := realtimeInput("publish")
		mutation(&in)
		_, err := invoker.InvokeCoreCapability(f.api, in)
		require.Error(t, err)
	}
}
func readRealtimeTestFrame(t *testing.T, scanner *bufio.Scanner) (string, map[string]json.RawMessage) {
	t.Helper()
	kind := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			kind = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") {
			var value map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &value))
			return kind, value
		}
	}
	require.NoError(t, scanner.Err())
	t.Fatal("REALTIME_FRAME_MISSING")
	return "", nil
}
func TestRealtimeStreamLiveRevocationAndPayload(t *testing.T) {
	f, invoker := realtimeFixture(t)
	ctx, cancel := context.WithTimeout(f.api, 5*time.Second)
	defer cancel()
	reader, err := invoker.InvokeCoreCapabilityStream(ctx, realtimeInput("subscribe"))
	require.NoError(t, err)
	defer reader.Close()
	scanner := bufio.NewScanner(reader)
	kind, _ := readRealtimeTestFrame(t, scanner)
	require.Equal(t, "ready", kind)
	in := realtimeInput("publish")
	_, err = invoker.InvokeCoreCapability(f.api, in)
	require.NoError(t, err)
	kind, frame := readRealtimeTestFrame(t, scanner)
	require.Equal(t, "event", kind)
	var event RealtimeEvent
	require.NoError(t, json.Unmarshal(frame["event"], &event))
	decoded, err := base64.StdEncoding.DecodeString(event.PayloadBase64)
	require.NoError(t, err)
	require.Contains(t, string(decoded), "9007199254740993")
	require.NoError(t, f.db.Where("api_key_uuid = ? AND scope = ?", f.key.UUID, "_scope.runtime.realtime.subscribe").Delete(&gm.IntegrationGatewayAPIKeyPermission{}).Error)
	_, err = invoker.InvokeCoreCapability(f.api, realtimeInput("publish"))
	require.NoError(t, err)
	kind, frame = readRealtimeTestFrame(t, scanner)
	require.Equal(t, "error", kind)
	require.Equal(t, `"REALTIME_GRANT_REQUIRED"`, string(frame["code"]))
}

// 该可选桥只连接隔离存储；身份头是受控夹具，不冒充真实 Gateway／STS 签发。
func TestRealtimeFrameworkBridgeFixture(t *testing.T) {
	file := os.Getenv("X07_CORE_BRIDGE_FILE")
	if file == "" {
		t.Skip("X07_OWNED_CORE_REQUIRED")
	}
	f, invoker := realtimeFixture(t)
	done := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fixture/stop" {
			done <- struct{}{}
			w.WriteHeader(200)
			return
		}
		var request struct {
			Capability string         `json:"capability_id"`
			Preferred  string         `json:"preferred_protocol"`
			Payload    map[string]any `json:"payload"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || request.Preferred != "core_internal" {
			w.WriteHeader(400)
			return
		}
		actor := f.api
		switch r.Header.Get("Authorization") {
		case "ApiKey fixture-key":
		case "Bearer fixture-sts":
			actor = f.sts
		default:
			w.WriteHeader(401)
			return
		}
		body, ok := request.Payload["body"].(map[string]any)
		if !ok {
			w.WriteHeader(400)
			return
		}
		call := cap.CoreCapabilityInvokeInput{CapabilityID: request.Capability, TenantUUID: cacheTenant, Endpoint: RealtimeEndpoint, Method: "INVOKE", Payload: request.Payload, Body: body}
		actor = reqctx.WithTenantUUID(actor, cacheTenant)
		ctx, cancel := context.WithCancel(actor)
		defer cancel()
		context.AfterFunc(r.Context(), cancel)
		if r.URL.Path == "/craft/v2/tenant/invocations/stream" {
			stream, err := invoker.InvokeCoreCapabilityStream(ctx, call)
			if err != nil {
				w.WriteHeader(dto.StatusCode(err))
				return
			}
			defer stream.Close()
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			buffer := make([]byte, 16384)
			for {
				n, err := stream.Read(buffer)
				if n > 0 {
					_, e := w.Write(buffer[:n])
					if e != nil {
						return
					}
					w.(http.Flusher).Flush()
				}
				if err != nil {
					if err != io.EOF && ctx.Err() == nil {
						t.Log("REALTIME_FIXTURE_STREAM_CLOSED")
					}
					return
				}
			}
		}
		if r.URL.Path != "/craft/v2/tenant/invocations" {
			w.WriteHeader(404)
			return
		}
		out, err := invoker.InvokeCoreCapability(ctx, call)
		if err != nil {
			w.WriteHeader(dto.StatusCode(err))
			_ = json.NewEncoder(w).Encode(map[string]any{"error_code": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"status": "completed", "protocol_used": "core_internal", "fallback_used": false, "payload": out}})
	}))
	t.Cleanup(server.Close)
	t.Logf("owned_realtime_fixture_listener=%s pid=%d", server.Listener.Addr(), os.Getpid())
	require.NoError(t, os.WriteFile(file, []byte(server.URL), 0600))
	meta, _ := json.Marshal(map[string]string{"schema": coremodel.PowerXSchema})
	require.NoError(t, os.WriteFile(file+".meta", meta, 0600))
	select {
	case <-done:
	case <-time.After(180 * time.Second):
		t.Fatal("REALTIME_FIXTURE_TIMEOUT")
	}
}
