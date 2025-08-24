package bootstrap

import (
	"context"
	"github.com/ArtisanCloud/PowerX/config"
	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	auditrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/audit"
	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
	"strings"
	"time" // 👈 新增
)

type Deps struct {
	DB           *gorm.DB
	AuthUser     *authsvc.AuthService
	AuthCustomer *authsvc.AuthService
	MeService    *authsvc.MeService

	//Bus    eventbus.Publisher // 来自 pkg/corex/event_bus

	AuditSvc auditsvc.Service // 底层批量写库 + sink
	Auditor  auditsvc.Auditor // 门面，兼容 LogAPI/LogRBAC 等调用
}

func NewDeps(db *gorm.DB, cfg *config.Config) *Deps {
	// repos
	tenantRepo := infraiam.NewTenantRepository(db)
	userRepo := infraiam.NewUserRepository(db)
	memberRepo := infraiam.NewMemberRepository(db)
	credRepo := infraiam.NewCredentialRepository(db)
	roleRepo := infraiam.NewRoleRepository(db)
	mrRepo := infraiam.NewMemberRoleRepository(db)
	rtRepo := infraiam.NewRefreshTokenRepository(db)

	accessTTL, _ := time.ParseDuration(cfg.Auth.AccessTTLStr)
	refreshTTL, _ := time.ParseDuration(cfg.Auth.RefreshTTLStr)

	authUser := authsvc.NewAuthService(
		tenantRepo, userRepo, memberRepo, credRepo, roleRepo, mrRepo, rtRepo,
		[]byte(strings.TrimSpace(cfg.Auth.JWTSecret)),
		cfg.Auth.Issuer,
		cfg.Auth.AudienceUser, // 👈 user audience
		cfg.Auth.Platforms,
		accessTTL, refreshTTL,
	)

	authCustomer := authsvc.NewAuthService(
		tenantRepo, userRepo, memberRepo, credRepo, roleRepo, mrRepo, rtRepo,
		[]byte(strings.TrimSpace(cfg.Auth.JWTSecret)),
		cfg.Auth.Issuer,
		cfg.Auth.AudienceCustomer,
		cfg.Auth.Platforms,
		accessTTL, refreshTTL,
	)

	// --- Audit 初始化 ---
	dbRepo := auditrepo.NewAuditEventRepository(db) // 你已有的 GORM repo
	sinks := []auditsvc.Sink{&auditsvc.LoggerSink{L: pxlog.GetGlobalLogger()}}
	svc := auditsvc.NewService(dbRepo, sinks, auditsvc.Options{
		BatchSize: 200, BatchWait: 150 * time.Millisecond, MaxPayloadSize: 16 * 1024,
	})
	// 注册 GORM 回调
	auditsvc.RegisterAuditCallbacks(db, svc)

	aud := auditsvc.NewAuditor(svc)

	_ = svc.Emit(context.Background(), &dbm.AuditEvent{
		OccurredAt:   time.Now(),
		Source:       "selftest",
		Operation:    "BOOT",
		ResourceType: "system",
		ResourceID:   "bootstrap",
		Outcome:      "SUCCESS",
		Severity:     "INFO",
		Meta:         []byte(`{"msg":"audit self test"}`),
	})

	// --- AuthService ---
	meSvc := authsvc.NewMeService(db)

	return &Deps{
		DB:           db,
		AuthUser:     authUser,
		AuthCustomer: authCustomer,
		MeService:    meSvc,
		AuditSvc:     svc,
		Auditor:      aud,
	}
}
