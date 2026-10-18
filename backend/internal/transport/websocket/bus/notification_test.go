package bus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	notifications "github.com/ArtisanCloud/PowerX/internal/service/notifications"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	ef "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type notificationGate struct {
	base             Authorizer
	block            atomic.Bool
	once             sync.Once
	waiting, release chan struct{}
}

func (g *notificationGate) Authorize(ctx context.Context, client *Client, topic string) error {
	if g.block.Load() {
		g.once.Do(func() { close(g.waiting) })
		<-g.release
	}
	return g.base.Authorize(ctx, client, topic)
}
func (g *notificationGate) AuthorizePublish(context.Context, PublishAuthorizeInput) error { return nil }

func TestNotificationFramesAreMemberScopedAndRevalidatedAtWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/iam.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	require.NoError(t, db.AutoMigrate(&im.User{}, &im.Member{}, &tm.Tenant{}))
	tenant, other := uuid.NewString(), uuid.NewString()
	for _, id := range []string{tenant, other} {
		require.NoError(t, db.Create(&tm.Tenant{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.MustParse(id)}, Key: id, Name: id, Status: 1}).Error)
	}
	members := map[string]im.Member{}
	for _, name := range []string{"a", "b", "root", "other"} {
		user := im.User{Email: name + "@notification.test", Status: 1, IsRoot: name == "root"}
		require.NoError(t, db.Create(&user).Error)
		tenantID := tenant
		if name == "other" {
			tenantID = other
		}
		m := im.Member{TenantUUID: tenantID, UserID: user.ID, Username: name, Status: 1}
		require.NoError(t, db.Create(&m).Error)
		members[name] = m
	}
	hub := NewHub()
	// Use the production authorizer without a per-member Topic ACL. Personal
	// notifications must be available to active authenticated recipients.
	gate := &notificationGate{waiting: make(chan struct{}), release: make(chan struct{}), base: NewDefaultAuthorizerWithOptions(DefaultAuthorizerOptions{
		DB:         db,
		TopicStore: &mockTopicLookup{topic: &ef.TopicDefinition{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.New()}, TenantKey: "global", Namespace: "_topic.system", Name: "notification", Status: 1, Lifecycle: ef.TopicLifecycleActive}},
		ACLStore:   &mockACLStore{allowed: false},
	})}
	memberB := members["b"]
	memberCtx := reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{TenantUUID: tenant, MemberID: memberB.ID, UserID: memberB.UserID, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}})
	memberClient := &Client{TenantUUID: tenant, MemberID: memberB.ID, MemberUUID: memberB.UUID.String(), UserID: memberB.UserID}
	require.NoError(t, gate.base.Authorize(memberCtx, memberClient, eventbus.TopicSystemNotification))
	require.ErrorIs(t, gate.base.AuthorizePublish(memberCtx, PublishAuthorizeInput{TenantUUID: tenant, MemberID: memberB.ID, UserID: memberB.UserID, Topic: eventbus.TopicSystemNotification}), ErrPermissionDenied)
	require.ErrorIs(t, gate.base.Authorize(memberCtx, memberClient, "custom.progress"), ErrPermissionDenied)
	forgedClient := &Client{TenantUUID: tenant, MemberID: memberB.ID, MemberUUID: members["a"].UUID.String(), UserID: memberB.UserID}
	require.ErrorIs(t, gate.base.Authorize(memberCtx, forgedClient, eventbus.TopicSystemNotification), ErrMemberRequired)
	require.Error(t, gate.base.Authorize(context.Background(), memberClient, eventbus.TopicSystemNotification))
	handler := &Handler{hub: hub, authorizer: gate, recipients: notifications.NewRecipientAccess(db)}
	engine := gin.New()
	engine.GET("/ws", func(c *gin.Context) {
		m := members[c.GetHeader("X-Test-Member")]
		claims := &reqctx.CoreXClaims{TenantUUID: m.TenantUUID, MemberID: m.ID, UserID: m.UserID, IsRoot: m.Username == "root", RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
		ctx := reqctx.WithClaims(c.Request.Context(), claims)
		ctx = reqctx.WithTenantUUID(ctx, m.TenantUUID)
		ctx = reqctx.WithMemberID(ctx, m.ID)
		ctx = reqctx.WithUserID(ctx, m.UserID)
		c.Request = c.Request.WithContext(ctx)
		handler.ServeWS(c)
	})
	server := httptest.NewServer(engine)
	defer server.Close()
	dial := func(name string) (*websocket.Conn, chan dto.WSBusEnvelope) {
		conn, _, e := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws", http.Header{"X-Test-Member": []string{name}})
		require.NoError(t, e)
		t.Cleanup(func() { _ = conn.Close() })
		var env dto.WSBusEnvelope
		require.NoError(t, conn.ReadJSON(&env))
		require.Equal(t, dto.WSBusTypeWelcome, env.Type)
		require.NoError(t, conn.WriteJSON(dto.WSBusCommand{Type: dto.WSBusCmdSubscribe, Topic: eventbus.TopicSystemNotification}))
		require.NoError(t, conn.ReadJSON(&env))
		require.Equal(t, dto.WSBusTypeAck, env.Type)
		frames := make(chan dto.WSBusEnvelope, 8)
		go func() {
			defer close(frames)
			for {
				var e dto.WSBusEnvelope
				if conn.ReadJSON(&e) != nil {
					return
				}
				frames <- e
			}
		}()
		return conn, frames
	}
	_, a := dial("a")
	_, b := dial("b")
	_, b2 := dial("b")
	_, c := dial("root")
	_, d := dial("other")
	member := members["b"]
	hub.Publish(tenant, eventbus.TopicSystemNotification, map[string]any{"id": "personal", "userId": member.UUID.String()}, "")
	for _, frames := range []chan dto.WSBusEnvelope{b, b2} {
		select {
		case env := <-frames:
			require.Contains(t, string(env.Payload), "personal")
		case <-time.After(time.Second):
			t.Fatal("member did not receive notification")
		}
	}
	for _, frames := range []chan dto.WSBusEnvelope{a, c, d} {
		select {
		case env := <-frames:
			t.Fatalf("notification leaked: %s", env.Payload)
		case <-time.After(80 * time.Millisecond):
		}
	}
	// The service receipt uses the same member target. Both devices receive it;
	// the other planner, Root and other tenant receive nothing.
	hub.Publish(tenant, eventbus.TopicSystemNotification, map[string]any{"eventType": "notification_read", "payload_version": 1, "contract": "notification_read_receipt.v1", "tenant_uuid": tenant, "userId": member.UUID.String(), "notification_uuids": []string{uuid.NewString()}}, "")
	for _, frames := range []chan dto.WSBusEnvelope{b, b2} {
		select {
		case env := <-frames:
			require.Contains(t, string(env.Payload), "notification_read")
		case <-time.After(time.Second):
			t.Fatal("member device did not receive read receipt")
		}
	}
	for _, frames := range []chan dto.WSBusEnvelope{a, c, d} {
		select {
		case env := <-frames:
			t.Fatalf("read receipt leaked: %s", env.Payload)
		case <-time.After(80 * time.Millisecond):
		}
	}
	hub.Publish(tenant, eventbus.TopicSystemNotification, map[string]any{"id": "bad", "userId": "invalid-target"}, "")
	for _, frames := range []chan dto.WSBusEnvelope{a, b, b2, c, d} {
		require.Empty(t, frames)
	}
	// Publish is the same adapter used by OpenAPI, Admin and TaskBus. Hold a
	// queued frame in authorization, revoke IAM, then let the writer continue.
	gate.block.Store(true)
	hub.Publish(tenant, eventbus.TopicSystemNotification, map[string]any{"id": "queued", "userId": member.UUID.String()}, "")
	select {
	case <-gate.waiting:
	case <-time.After(time.Second):
		t.Fatal("writer did not reach queued frame")
	}
	require.NoError(t, db.Model(&im.Member{}).Where("id=?", member.ID).Update("status", 0).Error)
	close(gate.release)
	for _, frames := range []chan dto.WSBusEnvelope{b, b2} {
		select {
		case env, ok := <-frames:
			require.False(t, ok, "disabled connection leaked queued frame: %s", env.Payload)
		case <-time.After(time.Second):
			t.Fatal("disabled connection was not closed")
		}
	}
	gate.block.Store(false)
	hub.Publish(tenant, eventbus.TopicSystemNotification, map[string]any{"id": "broadcast", "userId": ""}, "")
	for _, frames := range []chan dto.WSBusEnvelope{a, c} {
		select {
		case env := <-frames:
			require.Contains(t, string(env.Payload), "broadcast")
		case <-time.After(time.Second):
			t.Fatal("tenant broadcast missing")
		}
	}
	select {
	case env := <-d:
		t.Fatalf("cross-tenant broadcast: %s", env.Payload)
	case <-time.After(80 * time.Millisecond):
	}
}
