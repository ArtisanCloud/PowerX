package bootstrap

import (
	"github.com/ArtisanCloud/PowerX/config"
	infraiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	"gorm.io/gorm"

	authsvc "github.com/ArtisanCloud/PowerX/pkg/corex/iam/service"
)

type Deps struct {
	DB   *gorm.DB
	Auth *authsvc.AuthService
}

func NewDeps(db *gorm.DB, cfg *config.Config) *Deps {
	// 3) 业务依赖（AuthService 等）
	userRepo := infraiam.NewUserRepository(db)
	credRepo := infraiam.NewCredentialRepository(db)
	memberRepo := infraiam.NewMemberRepository(db)
	roleRepo := infraiam.NewRoleRepository(db)
	mrRepo := infraiam.NewMemberRoleRepository(db)
	rtRepo := infraiam.NewRefreshTokenRepository(db)

	auth := authsvc.NewAuthService(
		userRepo, memberRepo, credRepo, roleRepo, mrRepo, rtRepo,
		[]byte(cfg.Auth.JWTSecret),
	)

	return &Deps{
		DB:   db,
		Auth: auth,
	}
}
