package audit

import (
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"math/rand"
	"strconv"
	"time"
)

func GinAudit(auditor Auditor) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ---- Correlation-ID 注入开始 ----
		cid := c.Request.Header.Get("X-Correlation-ID")
		if cid == "" {
			// 你也可以替换为 uuid.New().String()
			cid = strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + strconv.Itoa(rand.Int())
		}
		ctxWithCID := WithCorrelationID(c.Request.Context(), cid)
		c.Request = c.Request.WithContext(ctxWithCID)
		c.Writer.Header().Set("X-Correlation-ID", cid)
		// ---- Correlation-ID 注入结束 ----

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

		tenantUUID := reqctx.GetTenantUUID(c.Request.Context())
		_ = auditor.(*serviceAuditor).svc.Emit(c.Request.Context(), &dbm.AuditEvent{
			OccurredAt:    time.Now(),
			TenantUUID:    tenantUUID,
			Source:        "http",
			Operation:     "API_CALL",
			ResourceType:  "core.api",
			ResourceID:    c.Request.Method + " " + fp,
			Outcome:       httpOutcome(status),
			Severity:      sevByHTTP(status),
			ClientIP:      ipPtr,
			ClientUA:      c.Request.UserAgent(),
			CorrelationID: CorrelationIDFromContext(c.Request.Context()), // ★ 回填
			Meta: mustJSON(map[string]any{
				"status":     status,
				"latency_ms": time.Since(start).Milliseconds(),
			}),
		})
	}
}
