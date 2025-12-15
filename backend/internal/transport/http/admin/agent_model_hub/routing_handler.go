package agentmodelhub

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	appshared "github.com/ArtisanCloud/PowerX/internal/app/shared"
	amhinst "github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/instrumentation"
	amhshared "github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/shared"
	modelrouting "github.com/ArtisanCloud/PowerX/internal/service/model_routing"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

// RoutingHandler exposes HTTP endpoints for routing policy governance.
type RoutingHandler struct {
	svc *modelrouting.Service
}

func NewRoutingHandler(deps *appshared.Deps) *RoutingHandler {
	if deps == nil || deps.DB == nil {
		return &RoutingHandler{}
	}
	svc := modelrouting.NewService(modelrouting.Options{
		Options: amhshared.Options{
			DB:              deps.DB,
			Cache:           cache.NewMemoryCache(),
			AuditSvc:        deps.AuditSvc,
			Instrumentation: amhinst.NewInstrumentation(nil, nil),
		},
	})
	return &RoutingHandler{svc: svc}
}

func (h *RoutingHandler) publishPolicy(c *gin.Context) {
	if h.svc == nil {
		dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "routing service unavailable", nil)
		return
	}
	tenantUUID, ok := requireTenantUUID(c)
	if !ok {
		return
	}
	var req applyPolicyRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	env := strings.TrimSpace(req.Env)
	if env == "" {
		env = "default"
	}
	rulesJSON, err := json.Marshal(req.Rules)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "invalid rules format", err)
		return
	}
	fallbackJSON, err := json.Marshal(req.FallbackChain)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "invalid fallbackChain format", err)
		return
	}
	input := modelrouting.PolicyInput{
		TenantScope:        tenantUUID,
		Rules:              datatypes.JSON(rulesJSON),
		FallbackChain:      datatypes.JSON(fallbackJSON),
		SafeModeThresholds: datatypes.JSONMap(req.SafeModeThresholds),
		ApprovalRecord:     datatypes.JSONMap(req.ApprovalConfig),
		Status:             "draft",
	}
	policy, err := h.svc.UpsertPolicyVersion(c.Request.Context(), env, input)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dtoRequest.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{
		"policy": buildPolicyDTO(policy),
	})
}

func (h *RoutingHandler) rollbackPolicy(c *gin.Context) {
	if h.svc == nil {
		dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "routing service unavailable", nil)
		return
	}
	tenantUUID, ok := requireTenantUUID(c)
	if !ok {
		return
	}
	var req rollbackPolicyRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	env := strings.TrimSpace(req.Env)
	if env == "" {
		env = "default"
	}
	policy, err := h.svc.RollbackPolicy(c.Request.Context(), env, tenantUUID, req.TargetVersion)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"policy": buildPolicyDTO(policy),
	})
}

func (h *RoutingHandler) routeTask(c *gin.Context) {
	if h.svc == nil {
		dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "routing service unavailable", nil)
		return
	}
	tenantUUID, ok := requireTenantUUID(c)
	if !ok {
		return
	}
	var req routeDecisionRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	env := strings.TrimSpace(req.Env)
	if env == "" {
		env = "default"
	}
	result, err := h.svc.DecideRoute(c.Request.Context(), env, tenantUUID, req.TaskContext)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"policyVersion":     result.PolicyVersion,
		"primaryProviderId": result.PrimaryProviderID,
		"fallbackChain":     result.FallbackChain,
		"traceId":           result.TraceID,
		"fallbackUsed":      result.FallbackUsed,
		"matchedRule":       result.MatchedRulePattern,
		"safeMode":          result.SafeMode,
	})
}

func (h *RoutingHandler) updatePolicyStatus(c *gin.Context) {
	if h.svc == nil {
		dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "routing service unavailable", nil)
		return
	}
	tenantUUID, ok := requireTenantUUID(c)
	if !ok {
		return
	}
	var req updatePolicyStatusRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	env := strings.TrimSpace(req.Env)
	if env == "" {
		env = "default"
	}
	target := strings.TrimSpace(req.TargetStatus)
	if target == "" {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "targetStatus required", nil)
		return
	}
	approvalUpdate, err := req.buildApproval()
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	input := modelrouting.StatusUpdateInput{
		TargetStatus: target,
		Reason:       strings.TrimSpace(req.Reason),
		Actor:        strings.TrimSpace(req.Actor),
		Approval:     approvalUpdate,
	}
	policy, err := h.svc.UpdatePolicyStatus(c.Request.Context(), env, tenantUUID, req.Version, input)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"policy": buildPolicyDTO(policy),
	})
}

func (h *RoutingHandler) toggleSafeMode(c *gin.Context) {
	if h.svc == nil {
		dtoRequest.ResponseError(c, http.StatusServiceUnavailable, "routing service unavailable", nil)
		return
	}
	tenantUUID, ok := requireTenantUUID(c)
	if !ok {
		return
	}
	var req safeModeToggleRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	env := strings.TrimSpace(req.Env)
	if env == "" {
		env = "default"
	}
	var ttl time.Duration
	if req.TTLSeconds > 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}
	state, err := h.svc.ToggleSafeMode(
		c.Request.Context(),
		env,
		tenantUUID,
		req.Enabled,
		ttl,
		req.Actor,
		req.Reason,
	)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, err.Error(), err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"state": buildSafeModeDTO(state),
	})
}

type applyPolicyRequest struct {
	Env                string           `json:"env"`
	Rules              []map[string]any `json:"rules" binding:"required"`
	FallbackChain      []string         `json:"fallbackChain"`
	SafeModeThresholds map[string]any   `json:"safeModeThresholds"`
	ApprovalConfig     map[string]any   `json:"approvalConfig"`
}

type rollbackPolicyRequest struct {
	Env           string `json:"env"`
	TargetVersion uint32 `json:"targetVersion"`
}

type routeDecisionRequest struct {
	Env         string         `json:"env"`
	TaskContext map[string]any `json:"taskContext" binding:"required"`
}

type updatePolicyStatusRequest struct {
	Env          string                 `json:"env"`
	Version      uint32                 `json:"version"`
	TargetStatus string                 `json:"targetStatus" binding:"required"`
	Reason       string                 `json:"reason"`
	Actor        string                 `json:"actor"`
	Approval     *approvalUpdatePayload `json:"approval"`
}

type approvalUpdatePayload struct {
	WorkflowID        string   `json:"workflowId"`
	RequestedBy       string   `json:"requestedBy"`
	RequiredApprovers uint32   `json:"requiredApprovers"`
	Approvers         []string `json:"approvers"`
	Outcome           string   `json:"outcome"`
	Notes             string   `json:"notes"`
	DecidedAt         string   `json:"decidedAt"`
}

func (req *updatePolicyStatusRequest) buildApproval() (*modelrouting.ApprovalUpdate, error) {
	if req.Approval == nil {
		return nil, nil
	}
	var decided time.Time
	if strings.TrimSpace(req.Approval.DecidedAt) != "" {
		parsed, err := time.Parse(time.RFC3339, req.Approval.DecidedAt)
		if err != nil {
			return nil, err
		}
		decided = parsed
	}
	return req.Approval.toService(decided), nil
}

func (a *approvalUpdatePayload) toService(decided time.Time) *modelrouting.ApprovalUpdate {
	if a == nil {
		return nil
	}
	approvers := make([]string, 0, len(a.Approvers))
	for _, name := range a.Approvers {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			approvers = append(approvers, trimmed)
		}
	}
	return &modelrouting.ApprovalUpdate{
		WorkflowID:        strings.TrimSpace(a.WorkflowID),
		RequestedBy:       strings.TrimSpace(a.RequestedBy),
		Approvers:         approvers,
		Outcome:           strings.TrimSpace(a.Outcome),
		Notes:             strings.TrimSpace(a.Notes),
		RequiredApprovers: a.RequiredApprovers,
		DecidedAt:         decided,
	}
}

type safeModeToggleRequest struct {
	Env        string `json:"env"`
	Enabled    bool   `json:"enabled"`
	Reason     string `json:"reason"`
	Actor      string `json:"actor"`
	TTLSeconds uint32 `json:"ttlSeconds"`
}

func jsonRaw(raw datatypes.JSON) any { return decodeJSON(raw, []any{}) }
