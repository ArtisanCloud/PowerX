package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver/local"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	svc "github.com/ArtisanCloud/PowerX/internal/service/media"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestVariantHTTPTransferWithCredentialGrant(t *testing.T) {
	runVariantHTTPTransfer(t, false)
}

func TestVariantHTTPTransferPostgres(t *testing.T) {
	if os.Getenv("POWERX_CONTRACT_TEST_POSTGRES_DSN") == "" {
		t.Skip("POWERX_CONTRACT_TEST_POSTGRES_DSN")
	}
	runVariantHTTPTransfer(t, true)
}

func runVariantHTTPTransfer(t *testing.T, usePostgres bool) {
	t.Helper()
	previous := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = previous })
	var dialector gorm.Dialector = sqlite.Open("file:variant_http_" + uuid.NewString() + "?mode=memory&cache=shared")
	if usePostgres {
		dialector = postgres.Open(os.Getenv("POWERX_CONTRACT_TEST_POSTGRES_DSN"))
		core.PowerXSchema = "contract_variant_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	db, e := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, e)
	sqlDB, e := db.DB()
	require.NoError(t, e)
	t.Cleanup(func() { sqlDB.Close() })
	if usePostgres {
		// DDL and fixture rows live in an uncommitted, uniquely named schema.
		// Rollback removes only this test's objects, never a business schema.
		db = db.Begin()
		require.NoError(t, db.Error)
		t.Cleanup(func() { require.NoError(t, db.Rollback().Error) })
		require.NoError(t, db.Exec(`CREATE SCHEMA "`+core.PowerXSchema+`"`).Error)
	}
	// SQLite fixture strips PostgreSQL-only default casts, not production DDL.
	for _, model := range []any{&m.MediaAsset{}, &m.MediaAssetVariant{}} {
		if usePostgres {
			break
		}
		stmt := &gorm.Statement{DB: db}
		require.NoError(t, stmt.Parse(model))
		for _, field := range stmt.Schema.Fields {
			field.DefaultValue = strings.TrimSuffix(field.DefaultValue, "::jsonb")
			if strings.Contains(strings.ToLower(field.TagSettings["INDEX"]), "gin") {
				delete(field.TagSettings, "INDEX")
			}
		}
	}
	require.NoError(t, db.AutoMigrate(&capm.CapabilityRecord{}, &capm.CapabilityRegistration{}, &setting.PluginInstanceConfig{}, &gw.IntegrationGatewayAPIKey{}, &gw.IntegrationGatewayAPIKeyPermission{}, &m.MediaAsset{}, &m.MediaAssetVariant{}))
	if usePostgres {
		require.NoError(t, db.AutoMigrate(&m.MediaAsset{}, &m.MediaAssetVariant{}))
	}
	tenant := uuid.NewString()
	plugin := "plugin.variant.test"
	asset := uuid.New()
	variant := uuid.New()
	expiry := time.Now().Add(time.Hour)
	hash := sha256.Sum256([]byte("payload"))
	checksum := hex.EncodeToString(hash[:])
	require.NoError(t, db.Create(&m.MediaAsset{PowerUUIDModel: core.PowerUUIDModel{UUID: asset}, TenantUUID: tenant, Name: "fixture", Driver: "local", StorageKey: asset.String() + "/origin", UploadState: m.UploadStateReady}).Error)
	var persistedParent m.MediaAsset
	require.NoError(t, db.Where("uuid = ?", asset).First(&persistedParent).Error)
	require.Nil(t, persistedParent.OwnerSubjectUUID)
	legacy := &m.MediaAssetVariant{TenantUUID: tenant, AssetUUID: asset.String(), Variant: "historical", Name: "fixture", Driver: "local", StorageKey: asset.String() + "/historical"}
	require.NoError(t, db.Create(legacy).Error)
	require.Equal(t, m.UploadStateFailed, legacy.UploadState)
	if usePostgres {
		require.NoError(t, db.AutoMigrate(&m.MediaAssetVariant{}))
		var historical m.MediaAssetVariant
		require.NoError(t, db.Where("uuid = ?", legacy.UUID).First(&historical).Error)
		require.Equal(t, legacy.UUID, historical.UUID)
		require.Equal(t, m.UploadStateFailed, historical.UploadState)
	}
	require.NoError(t, db.Create(&m.MediaAssetVariant{PowerUUIDModel: core.PowerUUIDModel{UUID: variant}, TenantUUID: tenant, AssetUUID: asset.String(), Variant: "preview", Name: "fixture", Driver: "local", StorageKey: asset.String() + "/preview.txt", MimeType: "text/plain; charset=utf-8", SizeBytes: 7, UploadState: m.UploadStatePending, ExpectedChecksum: checksum, UploadExpiresAt: &expiry, TicketVersion: 1}).Error)
	capID := svc.MediaAssetsTransferCapabilityID
	require.NoError(t, db.Create(&capm.CapabilityRecord{CapabilityID: capID, PluginID: "com.powerx.core", PluginVersion: "v1", Status: "published"}).Error)
	require.NoError(t, db.Create(&capm.CapabilityRegistration{CapabilityID: capID, TenantUUID: tenant, ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	credential := &setting.PluginInstanceConfig{TenantUUID: tenant, PluginID: plugin, Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(`{"allowed_capabilities":["` + capID + `"]}`)}
	require.NoError(t, db.Create(credential).Error)
	key := &gw.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: 1, Name: "fixture", KeyPrefix: "pxk", KeyHash: "variant-test-hash", Status: "active"}
	require.NoError(t, db.Create(key).Error)
	require.NoError(t, db.Create(&gw.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.media.assets.transfer", Action: "transfer", ResourceType: "api", ResourcePattern: "assets", Effect: "allow"}).Error)
	driver, e := local.New(local.Options{BasePath: t.TempDir()})
	require.NoError(t, e)
	manager := mediamgr.New("local")
	manager.RegisterDriver(driver)
	service := svc.NewMediaService(db, nil, manager, nil, time.Hour)
	service.SetPublicResourceTokenSecret("variant-http-test-secret")
	engine := gin.New()
	// Only credential verification is injected; actual access service, repositories,
	// storage and transfer handlers run. This is not a live STS-signature test.
	engine.Use(func(c *gin.Context) {
		var claims *reqctx.CoreXClaims
		switch c.GetHeader("X-Test-Credential") {
		case "sts":
			claims = &reqctx.CoreXClaims{TenantUUID: tenant, PluginID: plugin, RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}}}
		case "key":
			claims = &reqctx.CoreXClaims{TenantUUID: tenant, Platforms: []string{"api_key"}}
			c.Set("auth_api_key_hash", key.KeyHash)
		}
		if claims != nil {
			c.Request = c.Request.WithContext(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), claims))
		}
		c.Next()
	})
	deps := &shared.Deps{DB: db, MediaSvc: service}
	registerHostContract(engine.Group("/api/v1"), deps)
	RegisterPublicResource(engine, deps)
	path := "/api/v1/tenant/media/assets/" + asset.String() + "/variants/" + variant.String()
	call := func(method, path, body, actor string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("X-Test-Credential", actor)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w
	}
	require.Equal(t, 401, call("POST", path+"/presign-upload", "{}", "").Code)
	for _, body := range []string{`null`, `{"tenant_uuid":""}`, `{"expires_in_seconds":60,"expires_in_seconds":90}`, `{"Expires_In_Seconds":60}`, `{"expires_in_seconds":59}`, `{"expires_in_seconds":3601}`} {
		require.Equal(t, 400, call("POST", path+"/presign-upload", body, "sts").Code, body)
	}
	require.Equal(t, 404, call("POST", strings.Replace(path, asset.String(), uuid.NewString(), 1)+"/presign-upload", "{}", "sts").Code)
	var beforePresign m.MediaAssetVariant
	require.NoError(t, db.Where("uuid = ?", variant).First(&beforePresign).Error)
	require.Equal(t, 200, call("POST", path+"/presign-upload", "{}", "key").Code)
	w := call("POST", path+"/presign-upload", "{}", "sts")
	require.Equal(t, 200, w.Code, w.Body.String())
	var afterPresign m.MediaAssetVariant
	require.NoError(t, db.Where("uuid = ?", variant).First(&afterPresign).Error)
	require.Equal(t, beforePresign.UpdatedAt, afterPresign.UpdatedAt)
	var ticket struct {
		Data struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ticket))
	upload := httptest.NewRequest("PUT", ticket.Data.URL, bytes.NewBufferString("payload"))
	for k, v := range ticket.Data.Headers {
		upload.Header.Set(k, v)
	}
	w = httptest.NewRecorder()
	engine.ServeHTTP(w, upload)
	require.Equal(t, 204, w.Code, w.Body.String())
	require.Equal(t, 422, call("POST", path+"/complete-upload", `{"checksum":"`+strings.Repeat("0", 64)+`"}`, "sts").Code)
	w = call("POST", path+"/complete-upload", `{"checksum":"`+checksum+`"}`, "sts")
	require.Equal(t, 200, w.Code, w.Body.String())
	w = call("POST", path+"/presign-download", "{}", "key")
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ticket))
	w = call("GET", ticket.Data.URL, "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	b, e := io.ReadAll(w.Body)
	require.NoError(t, e)
	require.Equal(t, "payload", string(b))
	require.NoError(t, db.Model(&gw.IntegrationGatewayAPIKeyPermission{}).Where("api_key_uuid = ?", key.UUID).Update("effect", "deny").Error)
	require.Equal(t, 403, call("POST", path+"/presign-download", "{}", "key").Code)
	require.NoError(t, db.Model(credential).Update("value_json", datatypes.JSON(`{"allowed_capabilities":[]}`)).Error)
	require.Equal(t, 403, call("POST", path+"/presign-download", "{}", "sts").Code)
	require.NoError(t, db.Model(&m.MediaAsset{}).Where("uuid = ?", asset).Update("upload_state", m.UploadStateDeleted).Error)
	require.Equal(t, 409, call("GET", ticket.Data.URL, "", "").Code)
}
