package bootstrap

import (
	"github.com/ArtisanCloud/PowerX/config"
	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	authsvc "github.com/ArtisanCloud/PowerX/pkg/corex/iam/service"
	"gorm.io/gorm"
	"strings"
	"time" // 👈 新增
)

type Deps struct {
	DB           *gorm.DB
	AuthUser     *authsvc.AuthService
	AuthCustomer *authsvc.AuthService
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

	return &Deps{
		DB:           db,
		AuthUser:     authUser,
		AuthCustomer: authCustomer,
	}
}
