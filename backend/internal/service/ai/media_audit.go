package ai

import (
	"context"
	"encoding/json"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ai"
	auditmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	"gorm.io/datatypes"
	"time"
)

func (s *MediaRuntime) SetAudit(audit auditsvc.Service) { s.audit = audit }
func (s *MediaRuntime) auditMedia(ctx context.Context, row *m.MediaTask, operation string) {
	if s.audit == nil || row == nil {
		return
	}
	meta, _ := json.Marshal(map[string]any{"task_uuid": row.UUID.String(), "trace_id": row.TraceID, "subject": row.Subject, "capability": row.Capability, "status": row.Status, "revision": row.Revision})
	_ = s.audit.Emit(ctx, &auditmodel.AuditEvent{OccurredAt: time.Now().UTC(), TenantUUID: row.TenantUUID, Source: "ai.media", Operation: operation, ResourceType: "ai.media.task", ResourceID: row.UUID.String(), Outcome: "SUCCESS", Severity: "INFO", Meta: datatypes.JSON(meta)})
}
