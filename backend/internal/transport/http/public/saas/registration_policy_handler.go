package saas

import (
	"errors"
	"net/http"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type RegistrationPolicyHandler struct {
	policy  *authsvc.RegistrationPolicyService
	request *authsvc.RegistrationRequestService
}

func NewRegistrationPolicyHandler(deps *shared.Deps) *RegistrationPolicyHandler {
	if deps == nil {
		return &RegistrationPolicyHandler{}
	}
	policy := authsvc.NewRegistrationPolicyService(deps.DB)
	return &RegistrationPolicyHandler{
		policy:  policy,
		request: authsvc.NewRegistrationRequestService(deps.DB, authsvc.WithRegistrationRequestPolicy(policy)),
	}
}

type SubmitRegistrationRequest struct {
	TenantKey        string `json:"tenant_key" binding:"omitempty,max=64"`
	TenantName       string `json:"tenant_name" binding:"required,min=2,max=128"`
	Plan             string `json:"plan" binding:"omitempty,max=64"`
	OwnerEmail       string `json:"owner_email" binding:"omitempty,email"`
	OwnerPhone       string `json:"owner_phone" binding:"omitempty,max=32"`
	OwnerDisplayName string `json:"owner_display_name" binding:"omitempty,max=128"`
	InviteCode       string `json:"invite_code" binding:"omitempty,max=128"`
	Channel          string `json:"channel" binding:"omitempty,max=64"`
	Campaign         string `json:"campaign" binding:"omitempty,max=128"`
}

func (h *RegistrationPolicyHandler) Effective(c *gin.Context) {
	if h == nil || h.policy == nil {
		dto.ResponseError(c, http.StatusInternalServerError, "registration_policy.service_not_configured", nil)
		return
	}
	policy, err := h.policy.GetActive(c.Request.Context())
	if err != nil {
		dto.ResponseError(c, registrationPolicyHTTPStatus(err), err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"policy": gin.H{
			"mode":                  policy.Mode,
			"requires_verification": policy.RequiresVerification,
			"requires_invite_code":  policy.RequiresInviteCode || policy.Mode == "invite_only",
			"requires_request":      policy.Mode == "waitlist" || policy.Mode == "approval_required",
			"requires_approval":     policy.RequiresRootApproval || policy.Mode == "approval_required",
		},
	})
}

func (h *RegistrationPolicyHandler) SubmitRequest(c *gin.Context) {
	if h == nil || h.request == nil {
		dto.ResponseError(c, http.StatusInternalServerError, "registration_request.service_not_configured", nil)
		return
	}
	var req SubmitRegistrationRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	out, err := h.request.Submit(c.Request.Context(), authsvc.RegistrationRequestSubmitInput{
		TenantName:       req.TenantName,
		TenantKey:        req.TenantKey,
		OwnerEmail:       req.OwnerEmail,
		OwnerPhone:       req.OwnerPhone,
		OwnerDisplayName: req.OwnerDisplayName,
		Plan:             req.Plan,
		Channel:          req.Channel,
		Campaign:         req.Campaign,
		IP:               c.ClientIP(),
		UserAgent:        c.Request.UserAgent(),
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, authsvc.ErrRegistrationPolicyActiveMissing) {
			status = http.StatusServiceUnavailable
		}
		dto.ResponseError(c, status, err.Error(), err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, out)
}
