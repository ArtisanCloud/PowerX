package agentmodelhub

import (
	"context"
	"net/http"
	"strings"

	appshared "github.com/ArtisanCloud/PowerX/internal/app/shared"
	amhinst "github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/instrumentation"
	amhshared "github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/shared"
	costquota "github.com/ArtisanCloud/PowerX/internal/service/cost_quota"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type CostHandler struct {
	svc *costquota.Service
}

func NewCostHandler(deps *appshared.Deps) *CostHandler {
	if deps == nil || deps.DB == nil {
		return &CostHandler{}
	}
	if err := deps.DB.AutoMigrate(&model.CostQuotaLedger{}); err != nil {
		logger.WarnF(context.Background(), "[cost_handler] auto-migrate ledger table failed: %v", err)
	}
	svc := costquota.NewService(costquota.Options{
		Options: amhshared.Options{
			DB:              deps.DB,
			Cache:           cache.NewMemoryCache(),
			AuditSvc:        deps.AuditSvc,
			Instrumentation: amhinst.NewInstrumentation(nil, nil),
		},
	})
	return &CostHandler{svc: svc}
}

func (h *CostHandler) reportUsage(c *gin.Context) {
	if h.svc == nil {
		dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "cost quota service unavailable", nil)
		return
	}
	tenantUUID, ok := requireTenantUUID(c)
	if !ok {
		return
	}
	var req usageReportRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	env := strings.TrimSpace(req.Env)
	if env == "" {
		env = "default"
	}
	input := costquota.UsageIngestInput{
		TenantUUID:   tenantUUID,
		BudgetPeriod: req.BudgetPeriod,
	}
	if strings.TrimSpace(input.BudgetPeriod) == "" {
		input.BudgetPeriod = "monthly"
	}
	if parsed, err := uuid.Parse(strings.TrimSpace(req.ProviderID)); err == nil {
		input.ProviderID = &parsed
	}
	for _, evt := range req.Events {
		input.Events = append(input.Events, costquota.UsageIngestEvent{
			CostUSD: evt.CostUSD,
			Tokens:  evt.Tokens,
		})
	}
	if _, err := h.svc.ProcessUsage(c.Request.Context(), env, input); err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dtoRequest.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{"ok": true})
}

func (h *CostHandler) getQuotaSnapshot(c *gin.Context) {
	if h.svc == nil {
		dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "cost quota service unavailable", nil)
		return
	}
	tenantUUID, ok := requireTenantUUID(c)
	if !ok {
		return
	}
	env := strings.TrimSpace(c.Query("env"))
	if env == "" {
		env = "default"
	}
	ledgers, err := h.svc.ListLedgers(c.Request.Context(), env, tenantUUID)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, err.Error(), err)
		return
	}
	quotas := make([]map[string]any, 0, len(ledgers))
	for _, ledger := range ledgers {
		status := quotaStatus(&ledger)
		quotas = append(quotas, map[string]any{
			"providerId": providerDisplayID(ledger.ProviderProfileID),
			"limit":      ledger.QuotaLimit,
			"usage":      ledger.UsageActual,
			"status":     status,
		})
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"tenant_uuid": tenantUUID,
		"quotas":      quotas,
	})
}

func (h *CostHandler) enforceAction(c *gin.Context) {
	if h.svc == nil {
		dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "cost quota service unavailable", nil)
		return
	}
	tenantUUID, ok := requireTenantUUID(c)
	if !ok {
		return
	}
	var req enforcementRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	env := strings.TrimSpace(req.Env)
	if env == "" {
		env = "default"
	}
	input := costquota.EnforcementInput{
		TenantUUID:  tenantUUID,
		Action:      req.Action,
		Reason:      req.Reason,
		TicketID:    req.TicketID,
		RequestedBy: req.RequestedBy,
	}
	if parsed, err := uuid.Parse(strings.TrimSpace(req.ProviderID)); err == nil {
		input.ProviderID = &parsed
	}
	if _, err := h.svc.EnforceAction(c.Request.Context(), env, input); err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
}

type usageReportRequest struct {
	Env          string             `json:"env"`
	ProviderID   string             `json:"providerId"`
	BudgetPeriod string             `json:"budgetPeriod"`
	Events       []usageReportEvent `json:"events" binding:"required"`
}

type usageReportEvent struct {
	TraceID   string  `json:"traceId"`
	Tokens    uint64  `json:"tokens"`
	CostUSD   float64 `json:"costUsd"`
	Timestamp string  `json:"timestamp"`
}

type enforcementRequest struct {
	Env         string `json:"env"`
	ProviderID  string `json:"providerId"`
	Action      string `json:"action" binding:"required"`
	Reason      string `json:"reason"`
	TicketID    string `json:"ticketId"`
	RequestedBy string `json:"requestedBy"`
}

func quotaStatus(ledger *model.CostQuotaLedger) string {
	if ledger == nil || ledger.QuotaLimit <= 0 {
		return "healthy"
	}
	usage := ledger.UsageActual / ledger.QuotaLimit
	switch {
	case usage >= 1:
		return "breached"
	case usage >= 0.9:
		return "warning"
	default:
		return "healthy"
	}
}

func providerDisplayID(id *uuid.UUID) string {
	if id == nil {
		return "tenant"
	}
	return id.String()
}
