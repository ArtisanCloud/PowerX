package iam

import (
	"errors"
	"net/http"
	"strconv"

	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type RegistrationAdminHandler struct {
	policy *authsvc.RegistrationPolicyService
	invite *authsvc.InviteCodeService
	req    *authsvc.RegistrationRequestService
}

func NewRegistrationAdminHandler(policy *authsvc.RegistrationPolicyService, invite *authsvc.InviteCodeService, req *authsvc.RegistrationRequestService) *RegistrationAdminHandler {
	return &RegistrationAdminHandler{policy: policy, invite: invite, req: req}
}

type RegistrationPolicyUpdateRequest struct {
	Mode                 string                                `json:"mode" binding:"required"`
	RequiresVerification bool                                  `json:"requires_verification"`
	RequiresInviteCode   bool                                  `json:"requires_invite_code"`
	RequiresRootApproval bool                                  `json:"requires_root_approval"`
	DailyTenantQuota     *int                                  `json:"daily_tenant_quota"`
	TotalTenantQuota     *int                                  `json:"total_tenant_quota"`
	Rules                []authsvc.RegistrationPolicyRuleInput `json:"rules"`
}

type ActivateRegistrationPolicyRequest struct {
	PolicyUUID string `json:"policy_uuid" binding:"required"`
}

type CreateInviteBatchRequest struct {
	Name                string   `json:"name" binding:"required,min=2,max=128"`
	MaxCodes            int      `json:"max_codes" binding:"required,min=1,max=10000"`
	MaxUsesPerCode      int      `json:"max_uses_per_code" binding:"omitempty,min=1,max=100"`
	AllowedPlan         string   `json:"allowed_plan" binding:"omitempty,max=64"`
	AllowedEmailDomains []string `json:"allowed_email_domains"`
	AllowedChannels     []string `json:"allowed_channels"`
}

type GenerateInviteCodesRequest struct {
	Count int `json:"count" binding:"required,min=1,max=10000"`
}

type DeleteInviteBatchesRequest struct {
	BatchUUIDs []string `form:"batch_uuid" binding:"required,min=1,max=100"`
}

type ReviewRegistrationRequest struct {
	RejectReasonCode string `json:"reject_reason_code" binding:"omitempty,max=64"`
}

func (h *RegistrationAdminHandler) GetPolicy(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	out, err := h.policy.GetActive(c.Request.Context())
	if err != nil {
		dto.ResponseError(c, adminRegistrationStatus(err), err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *RegistrationAdminHandler) ListPolicyHistory(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	items, err := h.policy.ListHistory(c.Request.Context(), queryLimit(c))
	if err != nil {
		dto.ResponseError(c, adminRegistrationStatus(err), err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items})
}

func (h *RegistrationAdminHandler) UpdateDraft(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	var req RegistrationPolicyUpdateRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	out, err := h.policy.CreateDraft(c.Request.Context(), authsvc.RegistrationPolicyUpsertInput{
		Mode:                 req.Mode,
		RequiresVerification: req.RequiresVerification,
		RequiresInviteCode:   req.RequiresInviteCode,
		RequiresRootApproval: req.RequiresRootApproval,
		DailyTenantQuota:     req.DailyTenantQuota,
		TotalTenantQuota:     req.TotalTenantQuota,
		Rules:                req.Rules,
		ActorUserUUID:        reqctx.GetUserUUID(c.Request.Context()),
	})
	if err != nil {
		dto.ResponseError(c, adminRegistrationStatus(err), err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *RegistrationAdminHandler) ActivatePolicy(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	var req ActivateRegistrationPolicyRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	out, err := h.policy.Activate(c.Request.Context(), req.PolicyUUID, reqctx.GetUserUUID(c.Request.Context()))
	if err != nil {
		dto.ResponseError(c, adminRegistrationStatus(err), err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *RegistrationAdminHandler) ListInviteBatches(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	items, err := h.invite.ListBatches(c.Request.Context(), queryLimit(c))
	if err != nil {
		dto.ResponseError(c, adminRegistrationStatus(err), err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items})
}

func (h *RegistrationAdminHandler) CreateInviteBatch(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	var req CreateInviteBatchRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	out, err := h.invite.CreateBatch(c.Request.Context(), authsvc.InviteBatchCreateInput{
		Name:                req.Name,
		MaxCodes:            req.MaxCodes,
		MaxUsesPerCode:      req.MaxUsesPerCode,
		AllowedPlan:         req.AllowedPlan,
		AllowedEmailDomains: req.AllowedEmailDomains,
		AllowedChannels:     req.AllowedChannels,
		ActorUserUUID:       reqctx.GetUserUUID(c.Request.Context()),
	})
	if err != nil {
		dto.ResponseError(c, adminRegistrationStatus(err), err.Error(), err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, out)
}

func (h *RegistrationAdminHandler) DeleteInviteBatches(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	var req DeleteInviteBatchesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	deleted, err := h.invite.DeleteBatches(c.Request.Context(), req.BatchUUIDs)
	if err != nil {
		dto.ResponseError(c, adminRegistrationStatus(err), err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"deleted": deleted})
}

func (h *RegistrationAdminHandler) GenerateInviteCodes(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	var req GenerateInviteCodesRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	codes, err := h.invite.GenerateCodes(c.Request.Context(), c.Param("batch_uuid"), req.Count)
	if err != nil {
		dto.ResponseError(c, adminRegistrationStatus(err), err.Error(), err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{"batch_uuid": c.Param("batch_uuid"), "plain_codes": codes})
}

func (h *RegistrationAdminHandler) ListInviteCodes(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	items, err := h.invite.ListCodes(c.Request.Context(), c.Param("batch_uuid"), queryInviteCodeLimit(c))
	if err != nil {
		dto.ResponseError(c, adminRegistrationStatus(err), err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items})
}

func (h *RegistrationAdminHandler) ResetMissingInviteCodePlaintext(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	items, err := h.invite.ResetMissingPlainCodes(c.Request.Context(), c.Param("batch_uuid"))
	if err != nil {
		dto.ResponseError(c, adminRegistrationStatus(err), err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items})
}

func (h *RegistrationAdminHandler) PauseInviteBatch(c *gin.Context) {
	h.setInviteBatchStatus(c, authsvcStatusPaused())
}

func (h *RegistrationAdminHandler) RevokeInviteBatch(c *gin.Context) {
	h.setInviteBatchStatus(c, authsvcStatusRevoked())
}

func (h *RegistrationAdminHandler) setInviteBatchStatus(c *gin.Context, status string) {
	if !requireRoot(c) {
		return
	}
	out, err := h.invite.SetBatchStatus(c.Request.Context(), c.Param("batch_uuid"), status, reqctx.GetUserUUID(c.Request.Context()))
	if err != nil {
		dto.ResponseError(c, adminRegistrationStatus(err), err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *RegistrationAdminHandler) ListRequests(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	items, err := h.req.List(c.Request.Context(), c.Query("status"), queryLimit(c))
	if err != nil {
		dto.ResponseError(c, adminRegistrationStatus(err), err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items})
}

func (h *RegistrationAdminHandler) ApproveRequest(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	out, err := h.req.Approve(c.Request.Context(), authsvc.RegistrationRequestReviewInput{
		RequestUUID:      c.Param("request_uuid"),
		ReviewerUserUUID: reqctx.GetUserUUID(c.Request.Context()),
	})
	if err != nil {
		dto.ResponseError(c, adminRegistrationStatus(err), err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *RegistrationAdminHandler) RejectRequest(c *gin.Context) {
	if !requireRoot(c) {
		return
	}
	var req ReviewRegistrationRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	out, err := h.req.Reject(c.Request.Context(), authsvc.RegistrationRequestReviewInput{
		RequestUUID:      c.Param("request_uuid"),
		ReviewerUserUUID: reqctx.GetUserUUID(c.Request.Context()),
		RejectReasonCode: req.RejectReasonCode,
	})
	if err != nil {
		dto.ResponseError(c, adminRegistrationStatus(err), err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func requireRoot(c *gin.Context) bool {
	if !reqctx.IsRoot(c.Request.Context()) {
		dto.ResponseError(c, http.StatusForbidden, "registration_policy.root_required", nil)
		return false
	}
	return true
}

func queryLimit(c *gin.Context) int {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func queryInviteCodeLimit(c *gin.Context) int {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "1000"))
	if err != nil || limit <= 0 {
		return 1000
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func adminRegistrationStatus(err error) int {
	switch {
	case errors.Is(err, authsvc.ErrRegistrationPolicyActiveMissing):
		return http.StatusNotFound
	case errors.Is(err, authsvc.ErrRegistrationPolicyInvalid),
		errors.Is(err, authsvc.ErrRegistrationInviteInvalid),
		errors.Is(err, authsvc.ErrRegistrationRequestInvalid):
		return http.StatusBadRequest
	case errors.Is(err, authsvc.ErrRegistrationInviteUnavailable),
		errors.Is(err, authsvc.ErrRegistrationRequestNotFound):
		return http.StatusNotFound
	case errors.Is(err, authsvc.ErrRegistrationRequestStateConflict),
		errors.Is(err, authsvc.ErrRegistrationInviteConsumed):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func authsvcStatusPaused() string {
	return "paused"
}

func authsvcStatusRevoked() string {
	return "revoked"
}
