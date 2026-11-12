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

func TestIdempotentHTTPRequiresSignedServiceGrantAndReplaysCommittedCreation(t *testing.T) {
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
	_, credential := host.Actor(t, db, tenant, "plugin.notification.http.test")
	require.NoError(t, db.Create(&cm.CapabilityRecord{CapabilityID: svc.CreateCapabilityID, PluginID: "core", PluginVersion: "1", Status: "published"}).Error)
	require.NoError(t, db.Create(&cm.CapabilityRegistration{CapabilityID: svc.CreateCapabilityID, TenantUUID: tenant, Status: "published", ContractRef: "v1", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	u := im.User{Email: "idempotent-http@fixture.test", Status: 1}
	require.NoError(t, db.Create(&u).Error)
	m := im.Member{TenantUUID: tenant, UserID: u.ID, Username: "recipient", Status: 1}
	require.NoError(t, db.Create(&m).Error)
	key := []byte("fixture_notification_http_signing_key")
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &reqctx.CoreXClaims{TenantUUID: tenant, PluginID: credential.PluginID, Scope: "access", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:" + credential.PluginID, Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}).SignedString(key)
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(middleware.APIKeyOrJwtMiddleware(db, key, "fixture-core", []string{"user"}, []string{"access"}, nil, middleware.WithExtraTokenChecks(middleware.TokenCheck{Issuer: "powerx-sts", Audiences: []string{"powerx:api"}})))
	RegisterTenantRoutes(engine.Group("/api/v2"), &shared.Deps{DB: db, Notifications: svc.NewService(db)})
	call := func(body string, signed bool) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v2/notifications/idempotent", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		if signed {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		return recorder
	}
	body := `{"idempotency_key":"crm.approval.wait24.http","member_uuid":"` + m.UUID.String() + `","title":"fixture_title","content":"fixture_content","metadata":{"amount":9999999999999999.99}}`
	require.Equal(t, 401, call(body, false).Code)
	require.Equal(t, 403, call(body, true).Code)
	host.Grant(t, db, credential, svc.CreateCapabilityID)
	first := call(body, true)
	require.Equal(t, 200, first.Code, first.Body.String())
	var original struct {
		Data struct {
			Contract     string `json:"contract"`
			Replayed     bool   `json:"replayed"`
			Notification struct {
				ID string `json:"id"`
			} `json:"notification"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &original))
	require.Equal(t, svc.CreateReceiptContract, original.Data.Contract)
	require.False(t, original.Data.Replayed)
	// 丢弃首次响应后显式重试，仍只创建一条持久通知。
	again := call(body, true)
	require.Equal(t, 200, again.Code, again.Body.String())
	require.Contains(t, again.Body.String(), `"replayed":true`)
	require.Contains(t, again.Body.String(), original.Data.Notification.ID)
	require.Contains(t, again.Body.String(), "9999999999999999.99")
	require.Equal(t, 409, call(strings.Replace(body, "fixture_content", "fixture_changed", 1), true).Code)
	for _, field := range []string{"tenant_uuid", "owner", "headers", "endpoint"} {
		require.Equal(t, 400, call(strings.TrimSuffix(body, "}")+`,"`+field+`":"forbidden"}`, true).Code)
	}
	host.Grant(t, db, credential)
	require.Equal(t, 403, call(body, true).Code)
	var count int64
	require.NoError(t, db.Model(&nm.Notification{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
