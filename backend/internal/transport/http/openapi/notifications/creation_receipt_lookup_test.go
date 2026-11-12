package notifications

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	svc "github.com/ArtisanCloud/PowerX/internal/service/notifications"
	host "github.com/ArtisanCloud/PowerX/internal/testutil/runtime_host"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	nm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCreationLookupHTTPRequiresOwnReadGrantAndNeverChangesNotificationOrReceipt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := host.Database(t)
	models := []any{&nm.Notification{}, &nm.CreationReceipt{}, &im.User{}, &im.Member{}}
	for _, model := range models {
		stmt := &gorm.Statement{DB: db}
		require.NoError(t, stmt.Parse(model))
		for _, field := range stmt.Schema.Fields {
			field.DefaultValue = strings.TrimSuffix(field.DefaultValue, "::jsonb")
		}
	}
	require.NoError(t, db.AutoMigrate(models...))
	tenant := uuid.NewString()
	ctx, credential := host.Actor(t, db, tenant, "third.party.receipt.http")
	for _, id := range []string{svc.CreateCapabilityID, svc.CreationReceiptReadCapabilityID} {
		require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: id, PluginID: "core", PluginVersion: "1", Status: "published"}).Error)
		require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: id, TenantUUID: tenant, Status: "published", ContractRef: "v1", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	}
	u := im.User{Email: "lookup-http@fixture.test", Status: 1}
	require.NoError(t, db.Create(&u).Error)
	m := im.Member{TenantUUID: tenant, UserID: u.ID, Username: "recipient", Status: 1}
	require.NoError(t, db.Create(&m).Error)
	host.Grant(t, db, credential, svc.CreateCapabilityID)
	service := svc.NewService(db)
	created, err := service.CreateIdempotent(ctx, svc.IdempotentCreateInput{IdempotencyKey: "fixture.http.lookup", MemberUUID: m.UUID.String(), Title: "fixture_title", Content: "fixture_content"})
	require.NoError(t, err)
	key := []byte("fixture_receipt_lookup_signing_key")
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &reqctx.CoreXClaims{TenantUUID: tenant, PluginID: credential.PluginID, Scope: "access", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:" + credential.PluginID, Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}).SignedString(key)
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(middleware.APIKeyOrJwtMiddleware(db, key, "fixture-core", []string{"user"}, []string{"access"}, nil, middleware.WithExtraTokenChecks(middleware.TokenCheck{Issuer: "powerx-sts", Audiences: []string{"powerx:api"}})))
	RegisterTenantRoutes(engine.Group("/api/v2"), &shared.Deps{DB: db, Notifications: service})
	call := func(body string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/v2/notifications/creation-receipts/lookup", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if auth {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}
	body := `{"idempotency_key":"fixture.http.lookup","member_uuid":"` + m.UUID.String() + `"}`
	require.Equal(t, 401, call(body, false).Code)
	require.Equal(t, 403, call(body, true).Code)
	host.Grant(t, db, credential, svc.CreationReceiptReadCapabilityID)
	first := call(body, true)
	require.Equal(t, 200, first.Code, first.Body.String())
	require.Contains(t, first.Body.String(), created.Notification["id"])
	var view struct {
		Data svc.CreationReceiptLookup `json:"data"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &view))
	require.True(t, view.Data.Found)
	require.Equal(t, "present", view.Data.NotificationState)
	for _, field := range []string{"tenant_uuid", "plugin_id", "endpoint", "headers", "notification_uuid"} {
		require.Equal(t, 400, call(strings.TrimSuffix(body, "}")+`,"`+field+`":"forbidden"}`, true).Code)
	}
	require.Equal(t, 200, call(strings.Replace(body, "fixture.http.lookup", "fixture.missing", 1), true).Code)
	var count int64
	require.NoError(t, db.Model(&nm.Notification{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&nm.CreationReceipt{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	var original nm.Notification
	require.NoError(t, db.First(&original).Error)
	require.False(t, original.IsRead)
	require.Nil(t, original.ReadAt)
}
