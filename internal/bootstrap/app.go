// internal/bootstrap/app.go
package bootstrap

// 伪代码：按你的工程位置微调 import
import (
	"context"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/bootstrap"
	"github.com/ArtisanCloud/PowerX/internal/service/auth"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"time"
)

// internal/bootstrap/app.go

func BootstrapApp(ctx context.Context, cfg *config.Config) (*shared.Deps, error) {

	// 2. 初始化全局Logger
	// 使用配置中的日志配置初始化全局Logger
	logger.InitGlobalLogger(&cfg.LogConfig)
	// 测试全局Logger是否工作正常
	logger.Info(ctx, "🚀 全局Logger初始化成功")

	// 3) 初始化 GORM DB（按你的配置结构修改 getDB() 里读取字段）
	db, err := database.GetDB(&cfg.Database) // 👈 见下方实现
	if err != nil {
		logger.ErrorF(ctx, "初始化数据库失败: %v", err)
		return nil, err
	}

	// 1. 初始化缓存
	_, err = cache.InitCache(&cfg.Cache)
	if err != nil {
		logger.ErrorF(ctx, "初始化缓存失败: %s", err.Error())
		return nil, err
	}

	// 4. 初始化工具（agent_tools）
	err = bootstrap.InitAgentTools(ctx, &cfg.Agent, db)
	if err != nil {
		logger.ErrorF(ctx, "初始化工具失败: %s", err.Error())
		return nil, err
	}

	// 5. 初始化事件总线 / 插件订阅
	err = event_bus.InitEventBus()
	if err != nil {
		logger.ErrorF(ctx, "初始化事件总线失败: %s", err.Error())
	}

	// 6. 初始化依赖
	accessTTL, _ := time.ParseDuration(cfg.Auth.AccessTTLStr)
	refreshTTL, _ := time.ParseDuration(cfg.Auth.RefreshTTLStr)
	opts := &shared.DepsOptions{
		AuthUser: auth.AuthOptions{
			Issuer:     cfg.Auth.Issuer,
			Audience:   cfg.Auth.AudienceUser,
			Platforms:  cfg.Auth.Platforms,
			AccessTTL:  accessTTL,
			RefreshTTL: refreshTTL,
		},
		AuthCustomer: auth.AuthOptions{
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
