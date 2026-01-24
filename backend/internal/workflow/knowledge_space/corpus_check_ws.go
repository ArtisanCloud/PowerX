package knowledge_space

import (
	"context"
	"strings"

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
	wsbus.DefaultHub.Publish(tenant, wsbus.TopicKnowledgeCorpusCheck, job, reqctx.GetTraceID(ctx))
}
