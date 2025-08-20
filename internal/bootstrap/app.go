// internal/bootstrap/app.go
package bootstrap

// 伪代码：按你的工程位置微调 import
import (
	"context"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/ArtisanCloud/PowerX/services/agent/bootstrap"
)

// internal/bootstrap/app.go

func BootstrapApp(ctx context.Context, cfg *config.Config) (*Deps, error) {

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

	deps := NewDeps(db, cfg)

	return deps, nil
}
