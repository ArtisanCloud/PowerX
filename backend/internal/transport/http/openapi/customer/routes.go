package customer

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const customerAuthorizationHeader = "X-PowerX-Customer-Authorization"

type handler struct {
	memberships *customersvc.MembershipService
	tokens      *customersvc.CustomerTokenService
	auth        *customersvc.CustomerAuthService
}

func RegisterTenantRoutes(group *gin.RouterGroup, deps *shared.Deps) {
	if group == nil || deps == nil || deps.DB == nil || deps.AuthCustomer == nil {
		return
	}
	tokens := customersvc.NewCustomerTokenService(deps.DB, deps.AuthCustomer.JWTSecret, deps.AuthCustomer.Issuer, deps.AuthCustomer.AccessTTL)
	h := &handler{memberships: customersvc.NewMembershipService(deps.DB), tokens: tokens, auth: customersvc.NewCustomerAuthService(deps.DB, tokens)}
	g := group.Group("/tenant/customer")
	g.POST("/memberships:resolve", h.resolve)
	g.GET("/memberships", h.list)
	g.POST("/auth/validate", h.validate)
	g.POST("/auth/register", h.register)
	g.POST("/auth/login", h.login)
}

func (h *handler) resolve(c *gin.Context) {
	h.respondCurrent(c, customersvc.CustomerMembershipsDelegatedReadCapabilityID)
}
func (h *handler) list(c *gin.Context) {
	h.respondCurrent(c, customersvc.CustomerMembershipsDelegatedReadCapabilityID)
}
func (h *handler) validate(c *gin.Context) { h.respondCurrent(c, "com.corex.customer.auth.validate") }

type authRequest struct {
	TenantUUID string `json:"tenant_uuid"`
	Channel    string `json:"channel" binding:"required"`
	Credential struct {
		Type  string `json:"type" binding:"required"`
		Value string `json:"value" binding:"required"`
	} `json:"credential"`
}

func (h *handler) register(c *gin.Context) { h.issue(c, true) }
func (h *handler) login(c *gin.Context)    { h.issue(c, false) }
func (h *handler) issue(c *gin.Context, register bool) {
	var req authRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "CUSTOMER_INVALID_ARGUMENT", err)
		return
	}
	if strings.TrimSpace(req.TenantUUID) != "" || c.Request.URL.RawQuery != "" || c.GetHeader("X-Tenant-UUID") != "" {
		respondError(c, http.StatusBadRequest, "CUSTOMER_INVALID_ARGUMENT", customersvc.ErrCustomerUnauthorized)
		return
	}
	claims := reqctx.GetClaims(c.Request.Context())
	if claims == nil || !strings.EqualFold(claims.Issuer, "powerx-sts") || !hasAudience(claims.Audience, "powerx:api") || strings.TrimSpace(claims.PluginID) == "" {
		respondError(c, http.StatusUnauthorized, "CUSTOMER_UNAUTHORIZED", customersvc.ErrCustomerUnauthorized)
		return
	}
	tenantUUID, err := reqctx.CanonicalTenantUUID(claims.TenantUUID)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "CUSTOMER_UNAUTHORIZED", err)
		return
	}
	capability := "com.corex.customer.auth.login"
	if register {
		capability = "com.corex.customer.auth.register"
	}
	if err = h.memberships.AuthorizeDelegatedActorForCapability(c.Request.Context(), tenantUUID, claims.PluginID, capability); err != nil {
		respondMembershipError(c, err)
		return
	}
	if err = h.auth.AllowAttempt(c.Request.Context(), tenantUUID, claims.PluginID, req.Channel, req.Credential.Value, c.ClientIP()); err != nil {
		h.auth.RecordAttempt(c.Request.Context(), tenantUUID, claims.PluginID, req.Channel, req.Credential.Value, c.ClientIP(), capability, "CUSTOMER_FORBIDDEN", false)
		respondMembershipError(c, err)
		return
	}
	if req.Channel != customersvc.ShopifyStorefrontChannel || req.Credential.Type != "shopify_customer_access_token" {
		respondError(c, http.StatusBadRequest, "CUSTOMER_INVALID_ARGUMENT", customersvc.ErrCustomerCredentialInvalid)
		return
	}
	var pair customersvc.TokenPair
	var membership customersvc.Membership
	if register {
		pair, membership, err = h.auth.RegisterShopify(c.Request.Context(), tenantUUID, req.Credential.Value)
	} else {
		pair, membership, err = h.auth.LoginShopify(c.Request.Context(), tenantUUID, req.Credential.Value)
	}
	if err != nil {
		h.auth.RecordAttempt(c.Request.Context(), tenantUUID, claims.PluginID, req.Channel, req.Credential.Value, c.ClientIP(), capability, "CUSTOMER_CREDENTIAL_INVALID", false)
		respondAuthError(c, err)
		return
	}
	h.auth.RecordAttempt(c.Request.Context(), tenantUUID, claims.PluginID, req.Channel, req.Credential.Value, c.ClientIP(), capability, "", true)
	dto.ResponseSuccess(c, gin.H{
		"access_token":  pair.AccessToken,
		"refresh_token": pair.RefreshToken,
		"expires_in":    pair.ExpiresIn,
		"context": gin.H{
			"tenant_uuid":     membership.TenantUUID,
			"customer_uuid":   membership.CustomerUUID,
			"membership_uuid": membership.MembershipUUID,
			"status":          membership.Status,
			"roles":           membership.Roles,
			"scopes":          membership.Scopes,
			"expires_at":      membership.ExpiresAt,
			"authenticated":   true,
			"source":          "delegated",
		},
	})
}

func (h *handler) respondCurrent(c *gin.Context, capabilityID string) {
	if c.Request.Method == http.MethodPost && c.Request.ContentLength > 0 {
		respondError(c, http.StatusBadRequest, "CUSTOMER_UNAUTHORIZED", customersvc.ErrCustomerUnauthorized)
		return
	}
	if c.Request.URL.RawQuery != "" || c.GetHeader("X-Tenant-UUID") != "" || c.GetHeader("X-Customer-UUID") != "" || c.GetHeader("X-Membership-UUID") != "" {
		respondError(c, http.StatusBadRequest, "CUSTOMER_UNAUTHORIZED", customersvc.ErrCustomerUnauthorized)
		return
	}
	claims := reqctx.GetClaims(c.Request.Context())
	if claims == nil || !strings.EqualFold(claims.Issuer, "powerx-sts") || !hasAudience(claims.Audience, "powerx:api") || strings.TrimSpace(claims.PluginID) == "" {
		respondError(c, http.StatusUnauthorized, "CUSTOMER_UNAUTHORIZED", customersvc.ErrCustomerUnauthorized)
		return
	}
	tenantUUID, err := reqctx.CanonicalTenantUUID(claims.TenantUUID)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "CUSTOMER_UNAUTHORIZED", customersvc.ErrCustomerUnauthorized)
		return
	}
	if err := h.memberships.AuthorizeDelegatedActorForCapability(c.Request.Context(), tenantUUID, claims.PluginID, capabilityID); err != nil {
		respondMembershipError(c, err)
		return
	}
	raw := strings.TrimSpace(c.GetHeader(customerAuthorizationHeader))
	if !strings.HasPrefix(strings.ToLower(raw), "bearer ") {
		respondError(c, http.StatusUnauthorized, "CUSTOMER_UNAUTHORIZED", customersvc.ErrCustomerUnauthorized)
		return
	}
	customerClaims, err := h.tokens.Validate(c.Request.Context(), strings.TrimSpace(raw[7:]))
	if err != nil || customerClaims == nil {
		respondError(c, http.StatusUnauthorized, "CUSTOMER_UNAUTHORIZED", customersvc.ErrCustomerUnauthorized)
		return
	}
	customerUUID, customerErr := uuid.Parse(strings.TrimSpace(customerClaims.CustomerUUID))
	if customerErr != nil || customerClaims.TenantUUID != tenantUUID {
		respondError(c, http.StatusUnauthorized, "CUSTOMER_UNAUTHORIZED", customersvc.ErrCustomerUnauthorized)
		return
	}
	item, err := h.memberships.ResolveCurrent(c.Request.Context(), tenantUUID, customerUUID.String())
	if err != nil {
		respondMembershipError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"item": item})
}
func respondAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, customersvc.ErrCustomerCredentialInvalid):
		respondError(c, http.StatusUnauthorized, "CUSTOMER_CREDENTIAL_INVALID", err)
	case errors.Is(err, customersvc.ErrCustomerIdentityNotFound):
		respondError(c, http.StatusNotFound, "CUSTOMER_IDENTITY_NOT_FOUND", err)
	default:
		respondMembershipError(c, err)
	}
}

func hasAudience(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}
func respondMembershipError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, customersvc.ErrCustomerUnauthorized):
		respondError(c, http.StatusUnauthorized, "CUSTOMER_UNAUTHORIZED", err)
	case errors.Is(err, customersvc.ErrCustomerForbidden):
		respondError(c, http.StatusForbidden, "CUSTOMER_FORBIDDEN", err)
	case errors.Is(err, customersvc.ErrCustomerMembershipNotFound):
		respondError(c, http.StatusNotFound, "CUSTOMER_MEMBERSHIP_NOT_FOUND", err)
	case errors.Is(err, customersvc.ErrCustomerMembershipInactive):
		respondError(c, http.StatusForbidden, "CUSTOMER_MEMBERSHIP_INACTIVE", err)
	default:
		respondError(c, http.StatusServiceUnavailable, "CUSTOMER_UPSTREAM_DEPENDENCY", err)
	}
}
func respondError(c *gin.Context, status int, reason string, err error) {
	dto.ResponseErrorWithDetails(c, status, reason, err, map[string]interface{}{"reason_code": reason})
}
