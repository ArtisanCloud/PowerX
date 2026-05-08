package knowledge_space

import (
	"context"
	"strings"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	wsbus "github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

func publishCorpusCheckUpdate(ctx context.Context, job *models.CorpusCheckJob) {
	if job == nil {
		return
	}
	tenant := strings.TrimSpace(job.TenantUUID)
	if tenant == "" {
		return
	}
	wsbus.DefaultHub.PublishWithContext(ctx, tenant, eventbus.TopicKnowledgeCorpusCheckJob, job, reqctx.GetTraceID(ctx))
}
