package observability_ops

import (
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
)

func toAuditEvents(rows []auditrepoRow) []dbm.AuditEvent {
	out := make([]dbm.AuditEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, dbm.AuditEvent{
			OccurredAt:    row.OccurredAt,
			Source:        row.Source,
			Operation:     row.Operation,
			ResourceType:  row.ResourceType,
			ResourceID:    row.ResourceID,
			ActorUserID:   row.ActorUserID,
			TenantUUID:    row.TenantUUID,
			CorrelationID: row.Correlation,
			Outcome:       row.Outcome,
			Severity:      row.Severity,
			Meta:          row.Meta,
		})
	}
	return out
}
