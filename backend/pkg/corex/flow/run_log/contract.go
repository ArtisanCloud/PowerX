package run_log

import (
	"context"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/flow"
)

type RunLogger interface {
	PlanStart(ctx context.Context, e models.AgentTaskEvent)
	TaskStart(ctx context.Context, e models.AgentTaskEvent)
	TaskOK(ctx context.Context, e models.AgentTaskEvent)
	TaskErr(ctx context.Context, e models.AgentTaskEvent)
	PlanEnd(ctx context.Context, e models.AgentTaskEvent)
}
