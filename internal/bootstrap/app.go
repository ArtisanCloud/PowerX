// internal/bootstrap/app.go
package bootstrap

import (
	"context"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/bootstrap"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/internal/service/auth"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"log"
	"time"
)

func BootstrapApp(ctx context.Context, cfg *config.Config) (*shared.Deps, error) {

	// 初始化全局 Logger
	logger.InitGlobalLogger(&cfg.LogConfig)
	logger.Info(ctx, "🚀 全局 Logger 初始化成功")

	// 读取 Wrap 密钥
	if _, err := cfg.Server.ParseKey(); err != nil {
		log.Fatalf("读取 server.secret_key 失败: %v", err)
	} else {
		logger.Info(ctx, "Wrap 密钥已设置到全局")
	}

	// 初始化数据库连接（GORM）
	db, err := database.GetDB(&cfg.Database)
	if err != nil {
		logger.ErrorF(ctx, "初始化数据库失败: %v", err)
		return nil, err
	}

	// 初始化缓存
	_, err = cache.InitCache(&cfg.Cache)
	if err != nil {
		logger.ErrorF(ctx, "初始化缓存失败: %s", err.Error())
		return nil, err
	}

	// 加载 AI Catalog 配置
	if err := catalog.InitFromAppConfig(cfg.AI.Catalog, nil); err != nil {
		return nil, err
	}
	n := len(catalog.GetGlobalAIRegister().Providers("llm"))
	logger.InfoF(ctx, "[catalog] loaded providers: %d", n)

	// 初始化智能体工具（Agent Tools）
	err = bootstrap.InitAgentTools(ctx, &cfg.Agent, db)
	if err != nil {
		logger.ErrorF(ctx, "初始化工具失败: %s", err.Error())
		return nil, err
	}

	// 初始化事件总线（EventBus）
	err = event_bus.InitEventBus()
	if err != nil {
		logger.ErrorF(ctx, "初始化事件总线失败: %s", err.Error())
	}

	// 构建应用依赖（认证 / 审计等）
	accessTTL, _ := time.ParseDuration(cfg.Auth.AccessTTLStr)
	refreshTTL, _ := time.ParseDuration(cfg.Auth.RefreshTTLStr)
	opts := &shared.DepsOptions{
		AuthUser: auth.AuthOptions{
			JWTSecret:  []byte(cfg.Auth.JWTSecret),
			Issuer:     cfg.Auth.Issuer,
			Audience:   cfg.Auth.AudienceUser,
			Platforms:  cfg.Auth.Platforms,
			AccessTTL:  accessTTL,
			RefreshTTL: refreshTTL,
		},
		AuthCustomer: auth.AuthOptions{
			JWTSecret:  []byte(cfg.Auth.JWTSecret),
			Issuer:     cfg.Auth.Issuer,
			Audience:   cfg.Auth.AudienceCustomer,
			Platforms:  cfg.Auth.Platforms,
			AccessTTL:  accessTTL,
			RefreshTTL: refreshTTL,
		},
		Audit: auditsvc.AuditOptions{
			BatchSize: 200, BatchWait: 150 * time.Millisecond, MaxPayloadSize: 16 * 1024,
		},
	}

	deps := shared.NewDeps(db, opts)

	return deps, nil
}
