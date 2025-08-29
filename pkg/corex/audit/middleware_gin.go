package audit

import (
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"time"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	"github.com/gin-gonic/gin"
)

func GinAudit(auditor Auditor) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		fp := c.FullPath()
		if fp == "" {
			fp = c.Request.URL.Path
		}
		status := c.Writer.Status()

		// IP 只在非空时赋值（inet 列不接受空串）
		var ipPtr *string
		if ip := c.ClientIP(); ip != "" {
			ipPtr = &ip
		}

		_ = auditor.(*serviceAuditor).svc.Emit(c.Request.Context(), &dbm.AuditEvent{
			OccurredAt:   time.Now(),
			TenantID:     auth.GetTenantID(c),
			Source:       "http",
			Operation:    "API_CALL",
			ResourceType: "core.api",
			ResourceID:   c.Request.Method + " " + fp,
			Outcome:      httpOutcome(status),
			Severity:     sevByHTTP(status),
			ClientIP:     ipPtr,
			ClientUA:     c.Request.UserAgent(),
			Meta:         mustJSON(map[string]any{"status": status, "latency_ms": time.Since(start).Milliseconds()}),
		})
	}
}
