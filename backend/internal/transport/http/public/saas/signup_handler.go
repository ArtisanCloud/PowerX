package saas

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type SignupHandler struct {
	service  *authsvc.SaaSSignupService
	verifier *authsvc.SignupVerificationService
}

func NewSignupHandler(deps *shared.Deps) *SignupHandler {
	var svc *authsvc.SaaSSignupService
	var verifier *authsvc.SignupVerificationService
	if deps != nil {
		var opt authsvc.SaaSSignupOptions
		if cfg := config.GetGlobalConfig(); cfg != nil && cfg.FeatureGate.EnableSaaSSignupVerificationCode {
			verifier = authsvc.NewSignupVerificationService(authsvc.LocalSignupVerificationDriver{}, 10*time.Minute)
			opt.Verifier = verifier
		}
		svc = authsvc.NewSaaSSignupService(deps.DB, deps.AuthUser, opt)
	}
	return &SignupHandler{service: svc, verifier: verifier}
}

type SignupRequest struct {
	TenantKey        string `json:"tenant_key" binding:"omitempty,min=2,max=64"`
	TenantName       string `json:"tenant_name" binding:"required,min=2,max=128"`
	Plan             string `json:"plan" binding:"omitempty,max=64"`
	OwnerEmail       string `json:"owner_email" binding:"omitempty,email"`
	OwnerPhone       string `json:"owner_phone" binding:"omitempty,max=32"`
	OwnerPassword    string `json:"owner_password" binding:"required,min=6,max=64"`
	OwnerDisplayName string `json:"owner_display_name" binding:"omitempty,max=128"`
	VerificationCode string `json:"verification_code" binding:"omitempty,len=6"`
}

type SendVerificationCodeRequest struct {
	Contact string `json:"contact" binding:"required,min=6,max=128"`
}

func RegisterAPIRoutes(publicGroup *gin.RouterGroup, deps *shared.Deps) {
	if publicGroup == nil {
		return
	}
	h := NewSignupHandler(deps)
	publicGroup.POST("/public/saas/signup/verification-code", h.SendVerificationCode)
	publicGroup.POST("/public/saas/signup", h.Signup)
}

func saasSignupEnabled() bool {
	cfg := config.GetGlobalConfig()
	return cfg != nil && cfg.FeatureGate.EnableSaaSSignup
}

func (h *SignupHandler) SendVerificationCode(c *gin.Context) {
	if !saasSignupEnabled() {
		dtoRequest.ResponseError(c, http.StatusForbidden, "saas signup disabled", nil)
		return
	}
	if h == nil || h.verifier == nil {
		dtoRequest.ResponseError(c, http.StatusNotFound, "saas signup verification code disabled", nil)
		return
	}
	var req SendVerificationCodeRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	if err := h.verifier.Send(c.Request.Context(), req.Contact); err != nil {
		status := http.StatusBadRequest
		dtoRequest.ResponseError(c, status, err.Error(), err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"sent":    true,
		"contact": strings.TrimSpace(req.Contact),
	})
}

func (h *SignupHandler) Signup(c *gin.Context) {
	if !saasSignupEnabled() {
		dtoRequest.ResponseError(c, http.StatusForbidden, "saas signup disabled", nil)
		return
	}
	if h == nil || h.service == nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "saas signup service not configured", nil)
		return
	}
	var req SignupRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	result, err := h.service.Signup(c.Request.Context(), authsvc.SaaSSignupInput{
		TenantKey:        req.TenantKey,
		TenantName:       req.TenantName,
		Plan:             req.Plan,
		OwnerEmail:       req.OwnerEmail,
		OwnerPhone:       req.OwnerPhone,
		OwnerPassword:    req.OwnerPassword,
		OwnerDisplayName: req.OwnerDisplayName,
		VerificationCode: req.VerificationCode,
	})
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, authsvc.ErrSaaSSignupTenantKeyExists):
			status = http.StatusConflict
		case errors.Is(err, authsvc.ErrSaaSSignupTenantDomainExists):
			status = http.StatusConflict
		case errors.Is(err, authsvc.ErrSaaSSignupInvalidCredentials):
			status = http.StatusUnauthorized
		}
		dtoRequest.ResponseError(c, status, err.Error(), err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"token_type":    "Bearer",
		"access_token":  result.AccessToken,
		"refresh_token": result.RefreshToken,
		"expires_in":    int(h.service.AccessTTL().Seconds()),
		"scope":         "access",
		"context": gin.H{
			"is_root":             false,
			"current_tenant_uuid": result.Tenant.UUID.String(),
			"current_member_id":   result.Member.ID,
			"current_member_uuid": result.Member.UUID.String(),
			"user": gin.H{
				"id":           result.User.ID,
				"email":        result.User.Email,
				"phone":        result.User.Phone,
				"display_name": result.User.DisplayName,
				"status":       result.User.Status,
				"is_root":      result.User.IsRoot,
			},
			"members": []gin.H{{
				"tenant_uuid": result.Tenant.UUID.String(),
				"tenant_name": result.Tenant.Name,
				"member_id":   result.Member.ID,
				"member_uuid": result.Member.UUID.String(),
				"is_admin":    true,
				"is_owner":    true,
			}},
		},
	})
}
