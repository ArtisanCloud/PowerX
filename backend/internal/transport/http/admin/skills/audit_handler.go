package skills

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func newAuditHandler(
	traceRepo *skillrepo.SkillExecutionTraceRepository,
	auditRepo *skillrepo.SkillLifecycleAuditRepository,
) *auditHandler {
	if traceRepo == nil || auditRepo == nil {
		return nil
	}
	return &auditHandler{
		traceRepo: traceRepo,
		auditRepo: auditRepo,
	}
}

func (h *auditHandler) ListAudits(c *gin.Context) {
	skillID := strings.TrimSpace(c.Query("skill_id"))
	if skillID == "" {
		dto.ResponseError(c, http.StatusBadRequest, "skill_id is required", nil)
		return
	}
	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			limit = v
		}
	}
	items, err := h.auditRepo.ListBySkill(c.Request.Context(), skillID, limit)
	if err != nil {
		respondSkillError(c, err)
		return
	}
	data := make([]gin.H, 0, len(items))
	for i := range items {
		data = append(data, gin.H{
			"audit_id":      items[i].AuditID,
			"action":        items[i].Action,
			"skill_id":      items[i].SkillID,
			"version":       items[i].Version,
			"operator":      items[i].Operator,
			"tenant_scope":  items[i].TenantScope,
			"reason":        items[i].Reason,
			"result":        items[i].Result,
			"trace_id":      items[i].TraceID,
			"source":        items[i].Source,
			"error_summary": items[i].ErrorSummary,
			"created_at":    items[i].CreatedAt.Format(time.RFC3339),
		})
	}
	dto.ResponseSuccess(c, gin.H{"items": data, "total": len(data)})
}

func (h *auditHandler) GetTrace(c *gin.Context) {
	traceID := strings.TrimSpace(c.Param("traceId"))
	if traceID == "" {
		dto.ResponseError(c, http.StatusBadRequest, "trace_id is required", nil)
		return
	}
	record, err := h.traceRepo.GetByTraceID(c.Request.Context(), traceID)
	if err != nil {
		respondSkillError(c, err)
		return
	}
	tenantScope := strings.TrimSpace(c.Query("tenant_uuid"))
	if tenantScope == "" {
		tenantScope = strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	}
	if tenantScope != "" && !strings.EqualFold(strings.TrimSpace(record.TenantUUID), tenantScope) {
		dto.ResponseError(c, http.StatusNotFound, "trace not found", nil)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"trace_id":                 record.TraceID,
		"tenant_uuid":              record.TenantUUID,
		"skill_id":                 record.SkillID,
		"version":                  record.Version,
		"entrypoint":               record.Entrypoint,
		"protocol_used":            record.ProtocolUsed,
		"invoke_path":              record.InvokePath,
		"status":                   record.Status,
		"latency_ms":               record.LatencyMS,
		"error_code":               record.ErrorCode,
		"error_summary":            record.ErrorSummary,
		"request_payload_digest":   record.RequestPayloadDigest,
		"response_payload_digest":  record.ResponsePayloadDigest,
		"capability_id":            record.CapabilityID,
		"fallback_used":            record.FallbackUsed,
		"authorization_check_pass": record.AuthorizationCheckPass,
		"created_at":               record.CreatedAt.Format(time.RFC3339),
	})
}
