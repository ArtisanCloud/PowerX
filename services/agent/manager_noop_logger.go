package agent

// services/agent/manager_noop_logger.go

import (
	"context"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/flow"
)

type noopRunLogger struct{}

func (*noopRunLogger) PlanStart(context.Context, models.AgentTaskEvent) {}
func (*noopRunLogger) TaskStart(context.Context, models.AgentTaskEvent) {}
func (*noopRunLogger) TaskOK(context.Context, models.AgentTaskEvent)    {}
func (*noopRunLogger) TaskErr(context.Context, models.AgentTaskEvent)   {}
func (*noopRunLogger) PlanEnd(context.Context, models.AgentTaskEvent)   {}
