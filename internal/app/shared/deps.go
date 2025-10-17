package shared

// internal/bootstrap/deps.go

import (
	"context"
	"time"

	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	capRegPolicy "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	tenantsvc "github.com/ArtisanCloud/PowerX/internal/service/tenant"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	auditrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/audit"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
)

type Deps struct {
	DB           *gorm.DB
	ctx          *context.Context
	AuthUser     *authsvc.AuthService
	AuthCustomer *authsvc.AuthService
	MeService    *authsvc.MeService

	//Bus    eventbus.Publisher // 来自 pkg/corex/event_bus

	AuditSvc auditsvc.Service // 底层批量写库 + sink
	Auditor  auditsvc.Auditor // 门面，兼容 LogAPI/LogRBAC 等调用

	TenantSvc *tenantsvc.TenantService
	MediaMgr  *mediamgr.MediaManager
	MediaSvc  *mediasvc.MediaService
}

func NewDeps(db *gorm.DB, opts *DepsOptions) *Deps {
	ctx := context.Background()
	authUser := authsvc.NewAuthService(db, opts.AuthUser)
	authCustomer := authsvc.NewAuthService(db, opts.AuthCustomer)

	// --- Audit 初始化 ---
	dbRepo := auditrepo.NewAuditEventRepository(db) // 你已有的 GORM repo
	sinks := []auditsvc.Sink{&auditsvc.LoggerSink{L: pxlog.GetGlobalLogger()}}
	svc := auditsvc.NewService(dbRepo, sinks, opts.Audit)
	// 注册 GORM 回调
	auditsvc.RegisterAuditCallbacks(db, svc)

	aud := auditsvc.NewAuditor(svc)

	_ = svc.Emit(ctx, &dbm.AuditEvent{
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

	// --- TenantService ---
	tenantSvc := tenantsvc.NewTenantService(db, authUser)

	// --- Media Manager & Service ---
	mediaManager, mediaSvc := mediasvc.BuildMediaStack(ctx, db, svc, opts.Storage)

	permSvc := iamsvc.NewPermissionService(db)
	if err := capRegPolicy.EnsureAdminPermissions(ctx, permSvc); err != nil {
		pxlog.WarnF(ctx, "[capabilityRegistry] register permissions failed: %v", err)
	}

	return &Deps{
		DB:           db,
		TenantSvc:    tenantSvc,
		AuthUser:     authUser,
		AuthCustomer: authCustomer,
		MeService:    meSvc,
		AuditSvc:     svc,
		Auditor:      aud,
		MediaMgr:     mediaManager,
		MediaSvc:     mediaSvc,
	}
}
