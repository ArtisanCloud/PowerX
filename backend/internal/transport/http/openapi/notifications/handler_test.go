package notifications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	svc "github.com/ArtisanCloud/PowerX/internal/service/notifications"
	hosttest "github.com/ArtisanCloud/PowerX/internal/testutil/runtime_host"
	"github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	em "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	nm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type notificationTopic struct{}

func (notificationTopic) FindByComposite(context.Context, string, string, string) (*em.TopicDefinition, error) {
	return &em.TopicDefinition{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.New()}, TenantKey: "system"}, nil
}

type notificationACL struct{}

func (notificationACL) HasPermission(context.Context, string, uuid.UUID, string, string, time.Time) (bool, error) {
	return false, nil
}

// Real JWT verification -> existing HTTP route -> service/live grants -> DB ->
// WS recipient guard without a member Topic ACL. Fixture credentials and IAM
// do not claim installed browser acceptance.
func TestNotificationHTTPToMemberWebSocketWithSignedServiceToken(t *testing.T) {
	for _, prefix := range []string{"", "/api/v1", "/api/v2", "/core/custom", "/"} {
		t.Run(prefix, func(t *testing.T) {
			cfg := &config.Config{Server: config.ServerConfig{APIPrefix: prefix}}
			notificationHTTPToMemberWebSocketWithSignedServiceToken(t, strings.TrimRight(config.ResolveAPIPrefix(cfg), "/"))
		})
	}
}

func notificationHTTPToMemberWebSocketWithSignedServiceToken(t *testing.T, apiPrefix string) {
	t.Setenv("POWERX_RUNTIME_HOST_TEST_POSTGRES_DSN", "")
	gin.SetMode(gin.TestMode)
	db := hosttest.Database(t)
	stmt := &gorm.Statement{DB: db}
	require.NoError(t, stmt.Parse(&nm.Notification{}))
	for _, f := range stmt.Schema.Fields {
		f.DefaultValue = strings.TrimSuffix(f.DefaultValue, "::jsonb")
	}
	require.NoError(t, db.AutoMigrate(&nm.Notification{}, &im.User{}, &im.Member{}))
	tenant := uuid.NewString()
	_, credential := hosttest.Actor(t, db, tenant, "plugin.notice.test")
	require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: svc.CreateCapabilityID, PluginID: "core", PluginVersion: "1", Status: "published"}).Error)
	require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: svc.CreateCapabilityID, TenantUUID: tenant, ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: svc.ReadReceiptCapabilityID, PluginID: "core", PluginVersion: "1", Status: "published"}).Error)
	require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: svc.ReadReceiptCapabilityID, TenantUUID: tenant, Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	hosttest.Grant(t, db, credential, svc.CreateCapabilityID, svc.ReadReceiptCapabilityID)
	members := []im.Member{}
	for _, name := range []string{"a", "b", "c"} {
		u := im.User{Email: name + "@notice.test", Status: 1}
		require.NoError(t, db.Create(&u).Error)
		m := im.Member{TenantUUID: tenant, UserID: u.ID, Username: name, Status: 1}
		require.NoError(t, db.Create(&m).Error)
		members = append(members, m)
	}
	key := []byte("isolated-test-jwt-key-for-notification-delivery")
	sign := func(claims *reqctx.CoreXClaims) string {
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
		require.NoError(t, err)
		return token
	}
	serviceToken := sign(&reqctx.CoreXClaims{TenantUUID: tenant, PluginID: credential.PluginID, Scope: "access", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:" + credential.PluginID, Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}})
	old := bus.DefaultHub
	bus.DefaultHub = bus.NewHub()
	defer func() { bus.DefaultHub = old }()
	engine := gin.New()
	engine.Use(middleware.APIKeyOrJwtMiddleware(db, key, "test-core", []string{"user"}, []string{"access"}, nil, middleware.WithExtraTokenChecks(middleware.TokenCheck{Issuer: "powerx-sts", Audiences: []string{"powerx:api"}})))
	service := svc.NewService(db)
	RegisterTenantRoutes(engine.Group(apiPrefix), &shared.Deps{DB: db, Notifications: service})
	wsHandler := bus.NewHandlerWithOptions(bus.HandlerOptions{DB: db, TopicLookup: notificationTopic{}, ACLStore: notificationACL{}})
	engine.GET("/api/ws", wsHandler.ServeWS)
	server := httptest.NewServer(engine)
	defer server.Close()
	connections := []*websocket.Conn{}
	defer func() {
		for _, conn := range connections {
			_ = conn.Close()
		}
	}()
	frames := []chan dto.WSBusEnvelope{}
	for _, m := range members {
		token := sign(&reqctx.CoreXClaims{TenantUUID: tenant, MemberID: m.ID, UserID: m.UserID, MemberUUID: m.UUID.String(), Scope: "access", RegisteredClaims: jwt.RegisteredClaims{Issuer: "test-core", Subject: m.UUID.String(), Audience: jwt.ClaimStrings{"user"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}})
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/ws", http.Header{"Authorization": []string{"Bearer " + token}})
		require.NoError(t, err)
		connections = append(connections, conn)
		var env dto.WSBusEnvelope
		require.NoError(t, conn.ReadJSON(&env))
		require.NoError(t, conn.WriteJSON(dto.WSBusCommand{Type: dto.WSBusCmdSubscribe, Topic: eventbus.TopicSystemNotification}))
		require.NoError(t, conn.ReadJSON(&env))
		require.Equal(t, dto.WSBusTypeAck, env.Type)
		ch := make(chan dto.WSBusEnvelope, 2)
		frames = append(frames, ch)
		go func() {
			defer close(ch)
			for {
				var frame dto.WSBusEnvelope
				if conn.ReadJSON(&frame) != nil {
					return
				}
				ch <- frame
			}
		}()
	}
	post := func(target, token string, destinations ...string) *httptest.ResponseRecorder {
		destination := "/_p/plugin.test/admin/order?target=opaque"
		if len(destinations) > 0 {
			destination = destinations[0]
		}
		raw, _ := json.Marshal(map[string]any{"title": "AI Craft update", "content": "Open the plugin to view details", "member_uuid": target, "navigation": map[string]string{"label": "fixture_action", "path": destination}})
		r := httptest.NewRequest("POST", apiPrefix+"/notifications", strings.NewReader(string(raw)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}
	w := post(members[1].UUID.String(), serviceToken)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "view_business")
	var saved nm.Notification
	require.NoError(t, db.First(&saved).Error)
	require.Contains(t, string(saved.Actions), "opaque")
	select {
	case env := <-frames[1]:
		require.Contains(t, string(env.Payload), members[1].UUID.String())
		require.Contains(t, string(env.Payload), "view_business")
		require.Contains(t, string(env.Payload), "/_p/plugin.test/admin/order?target=opaque")
	case <-time.After(time.Second):
		t.Fatal("recipient did not receive created notice")
	}
	for _, ch := range []chan dto.WSBusEnvelope{frames[0], frames[2]} {
		select {
		case env := <-ch:
			t.Fatalf("personal payload leaked: %s", env.Payload)
		case <-time.After(80 * time.Millisecond):
		}
	}
	raw, _ := json.Marshal(svc.ReadReceiptInput{MemberUUID: members[1].UUID.String(), NotificationUUIDs: []string{saved.UUID.String()}})
	receiptRequest := httptest.NewRequest("POST", apiPrefix+"/notifications/read-receipts", strings.NewReader(string(raw)))
	receiptRequest.Header.Set("Content-Type", "application/json")
	receiptRequest.Header.Set("Authorization", "Bearer "+serviceToken)
	receiptResponse := httptest.NewRecorder()
	engine.ServeHTTP(receiptResponse, receiptRequest)
	require.Equal(t, 200, receiptResponse.Code, receiptResponse.Body.String())
	select {
	case event := <-frames[1]:
		require.Contains(t, string(event.Payload), "notification_read")
		require.Contains(t, string(event.Payload), saved.UUID.String())
	case <-time.After(time.Second):
		t.Fatal("NOTIFICATION_READ_EVENT_MISSING")
	}
	require.NoError(t, db.First(&saved, "uuid = ?", saved.UUID).Error)
	require.True(t, saved.IsRead)
	for _, ch := range []chan dto.WSBusEnvelope{frames[0], frames[2]} {
		select {
		case event := <-ch:
			t.Fatalf("NOTIFICATION_READ_LEAK:%s", event.Payload)
		case <-time.After(50 * time.Millisecond):
		}
	}
	w = post(members[1].UUID.String(), serviceToken, "//external.test")
	require.Equal(t, 400, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "NOTIFICATION_NAVIGATION_INVALID")
	w = post(uuid.NewString(), serviceToken)
	require.Equal(t, 400, w.Code, w.Body.String())
	hosttest.Grant(t, db, credential)
	w = post(members[1].UUID.String(), serviceToken)
	require.Equal(t, 403, w.Code, w.Body.String())
	var count int64
	require.NoError(t, db.Model(&nm.Notification{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
