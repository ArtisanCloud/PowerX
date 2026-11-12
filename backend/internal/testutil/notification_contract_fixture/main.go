// 仅供隔离 SDK/CRM 合同验收；固定合成身份，拒绝日常数据库，HTTP 仅监听 loopback 随机端口。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	gateway "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway"
	notices "github.com/ArtisanCloud/PowerX/internal/service/notifications"
	noticehttp "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/notifications"
	"github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	audit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	nm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/notification"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	cr "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	tr "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const tenant = "00000000-0000-4000-8000-000000000001"
const capability = "com.corex.notifications.create"
const receiptRead = "com.corex.notifications.creation_receipts.read"
const organization = "com.corex.iam.organization.service_read"
const adminWrite = "com.corex.iam.member_accounts.admin_write"

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "prepare" && os.Args[1] != "verify" && os.Args[1] != "serve") || os.Getenv("CRM_TEMP_CORE_DSN") == "" {
		panic("isolated_fixture_requires_explicit_dsn_and_prepare_verify_or_serve")
	}
	db, err := gorm.Open(postgres.Open(os.Getenv("CRM_TEMP_CORE_DSN")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	must(err)
	pool, err := db.DB()
	must(err)
	defer pool.Close()
	var actual string
	must(db.Raw("select current_database()").Scan(&actual).Error)
	if actual != "crm_notification_lab" {
		panic("isolated_database_required")
	}
	if os.Args[1] == "serve" {

		if os.Getenv("CRM_NOTIFICATION_FIXTURE_FILE") == "" {
			panic("isolated_fixture_output_required")
		}
		serveSDKFixture(db)
		return
	}
	if os.Args[1] == "prepare" {
		must(db.AutoMigrate(&im.Permission{}, &im.APIKeyProfile{}, &im.APIKeyProfilePermission{}, &gm.IntegrationGatewayAPIKey{}, &gm.IntegrationGatewayAPIKeyPermission{}, &cm.CapabilityRecord{}, &cm.CapabilityRegistration{}, &cm.AdapterEndpoint{}, &cm.RoutingPolicy{}, &cm.FallbackPlan{}, &audit.AuditEvent{}, &tm.Tenant{}, &setting.PluginInstanceConfig{}))
		must(db.Create(&tm.Tenant{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.MustParse(tenant)}, Key: "fixture", Name: "fixture", Status: 1}).Error)
		must(gateway.NewBaseCapabilitySeeder(gateway.BaseCapabilitySeederOptions{RecordRepo: cr.NewCapabilityRecordRepository(db, nil), RegistryRepo: cr.NewCapabilityRegistryRepository(db), TenantRepo: tr.NewTenantRepository(db), CapabilityIDs: []string{capability, receiptRead, organization, adminWrite}}).Ensure(context.Background()))
		key := gm.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: 1, Name: "fixture_key", KeyPrefix: "fixture", KeyHash: "fixture_notification_lab_hash", Status: "active"}
		must(db.Create(&key).Error)
		must(db.Create(&gm.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.notifications.create", Action: "create", ResourceType: "capability", ResourcePattern: "notifications", Effect: "allow"}).Error)
		must(db.Create(&gm.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.notifications.creation_receipts", Action: "read", ResourceType: "capability", ResourcePattern: "notifications", Effect: "allow"}).Error)
		must(db.Create(&gm.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.iam.organization.service_read", Action: "read", ResourceType: "capability", ResourcePattern: "iam_organization", Effect: "allow"}).Error)
		fmt.Println("prepared isolated catalog and explicit fixture key")
	} else {
		ctx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, Platforms: []string{"api_key"}}), "fixture_notification_lab_hash")
		access, err := cap.NewGrantStatusService(db).CurrentAccess(ctx)
		must(err)
		must(access.Require(capability))
		must(access.Require(receiptRead))
		must(access.Require(organization))
		if access.Require(adminWrite) == nil {
			panic("admin_write_must_not_be_service_accessible")
		}
		var rows []im.Permission
		must(db.Where("allow_api_key = ?", true).Find(&rows).Error)
		if len(rows) == 0 {
			panic("missing_service_permission")
		}
		fmt.Println("verified fixture grant-status and explicit service permissions")
	}
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}

func serveSDKFixture(db *gorm.DB) {
	must(db.AutoMigrate(&im.User{}, &im.Member{}, &nm.Notification{}, &nm.CreationReceipt{}))
	user := im.User{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.MustParse("00000000-0000-4000-8000-000000000003")}, Email: "sdk@fixture.test", Status: 1}
	must(db.Create(&user).Error)
	member := im.Member{PowerUUIDModel: core.PowerUUIDModel{UUID: uuid.MustParse("00000000-0000-4000-8000-000000000002")}, TenantUUID: tenant, UserID: user.ID, UserUUID: user.UUID.String(), Username: "sdk_fixture", Status: 1}
	must(db.Create(&member).Error)
	const plugin = "plugin.fixture.crm"
	raw, _ := json.Marshal(map[string]any{"client_id": plugin, "allowed_capabilities": []string{capability, receiptRead}})
	must(db.Create(&setting.PluginInstanceConfig{TenantUUID: tenant, PluginID: plugin, Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(raw)}).Error)
	key := []byte("fixture_cross_repo_notification_key")
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &reqctx.CoreXClaims{TenantUUID: tenant, PluginID: plugin, Scope: "access", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:" + plugin, Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute))}}).SignedString(key)
	must(err)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.APIKeyOrJwtMiddleware(db, key, "fixture-core", []string{"user"}, []string{"access"}, nil, middleware.WithExtraTokenChecks(middleware.TokenCheck{Issuer: "powerx-sts", Audiences: []string{"powerx:api"}})))
	noticehttp.RegisterTenantRoutes(engine.Group("/api/v2"), &shared.Deps{DB: db, Notifications: notices.NewService(db)})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	must(err)
	fixture, _ := json.Marshal(map[string]any{"contract": "isolated_notification_contract.v1", "base_url": "http://" + listener.Addr().String(), "tenant_uuid": tenant, "member_uuid": member.UUID.String(), "token": token, "pid": os.Getpid()})
	must(os.WriteFile(os.Getenv("CRM_NOTIFICATION_FIXTURE_FILE"), fixture, 0600))
	fmt.Printf("Owned SDK fixture pid=%d listen=%s\n", os.Getpid(), listener.Addr().String())
	server := &http.Server{Handler: engine, ReadHeaderTimeout: 3 * time.Second}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		panic(err)
	}
}
