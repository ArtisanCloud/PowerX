package bootstrap

import (
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	flowModel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/flow"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	flowrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/flow"
	runLog "github.com/ArtisanCloud/PowerX/pkg/corex/flow/run_log"
	"gorm.io/gorm"
)

func WireAgentRunLogger(db *gorm.DB) {
	// 1) 构造仓库（如果你已经有构造函数 NewAgentPlanRunRepository/NewAgentTaskEventRepository，也可以用它们）
	planRepo := &flowrepo.AgentPlanRunRepository{
		BaseRepository: baseRepo.NewBaseRepository[flowModel.AgentPlanRun](db),
	}
	eventRepo := &flowrepo.AgentTaskEventRepository{
		BaseRepository: baseRepo.NewBaseRepository[flowModel.AgentTaskEvent](db),
	}

	// 2) 创建 DBRunLogger（内部异步队列，buf 可按量调大）
	logger := runLog.NewDBRunLogger(planRepo, eventRepo, 2048)

	// 3) 注入到 Manager
	agent.GetAgentManager().SetRunLogger(logger)

	// 4) （可选）进程退出时优雅关闭
	// onShutdown(func() { logger.Close() })
}
