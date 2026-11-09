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

// RouteOption 调整客户委派路由的可选能力。
type RouteOption func(*customersvc.CustomerAuthService)

// WithClientApps 用客户端应用注册表启用小程序（wechat_miniapp 等）委派登录。
func WithClientApps(registry customersvc.ClientAppRegistry) RouteOption {
	return func(auth *customersvc.CustomerAuthService) { auth.WithClientApps(registry) }
}

func RegisterTenantRoutes(group *gin.RouterGroup, deps *shared.Deps, opts ...RouteOption) {
	if group == nil || deps == nil || deps.DB == nil || deps.AuthCustomer == nil {
		return
	}
	tokens := customersvc.NewCustomerTokenService(deps.DB, deps.AuthCustomer.JWTSecret, deps.AuthCustomer.Issuer, deps.AuthCustomer.AccessTTL)
	auth := customersvc.NewCustomerAuthService(deps.DB, tokens)
	for _, opt := range opts {
		opt(auth)
	}
	h := &handler{memberships: customersvc.NewMembershipService(deps.DB), tokens: tokens, auth: auth}
	g := group.Group("/tenant/customer")
	g.POST("/memberships:resolve", h.resolve)
	g.GET("/memberships", h.list)
	g.POST("/auth/validate", h.validate)
	g.POST("/auth/register", h.register)
	g.POST("/auth/login", h.login)
	g.POST("/phone:verify", h.verifyPhone)
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
	// AppID 是客户端应用（小程序）ID；channel 为小程序平台时必填，Core 据此在注册表中找到凭证并校验插件授权。
	AppID      string `json:"app_id"`
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
	var pair customersvc.TokenPair
	var membership customersvc.Membership
	switch {
	case req.Channel == customersvc.ShopifyStorefrontChannel && req.Credential.Type == "shopify_customer_access_token":
		if register {
			pair, membership, err = h.auth.RegisterShopify(c.Request.Context(), tenantUUID, req.Credential.Value)
		} else {
			pair, membership, err = h.auth.LoginShopify(c.Request.Context(), tenantUUID, req.Credential.Value)
		}
	case isMiniAppCredential(req.Channel, req.Credential.Type):
		if strings.TrimSpace(req.AppID) == "" {
			respondError(c, http.StatusBadRequest, "CUSTOMER_INVALID_ARGUMENT", customersvc.ErrCustomerClientAppNotFound)
			return
		}
		if register {
			pair, membership, err = h.auth.RegisterMiniApp(c.Request.Context(), tenantUUID, req.Channel, req.AppID, req.Credential.Value)
		} else {
			pair, membership, err = h.auth.LoginMiniApp(c.Request.Context(), tenantUUID, req.Channel, req.AppID, req.Credential.Value)
		}
	default:
		respondError(c, http.StatusBadRequest, "CUSTOMER_INVALID_ARGUMENT", customersvc.ErrCustomerCredentialInvalid)
		return
	}
	if err != nil {
		h.auth.RecordAttempt(c.Request.Context(), tenantUUID, claims.PluginID, req.Channel, req.Credential.Value, c.ClientIP(), capability, "CUSTOMER_CREDENTIAL_INVALID", false)
		respondAuthError(c, err)
		return
	}
	h.auth.RecordAttempt(c.Request.Context(), tenantUUID, claims.PluginID, req.Channel, req.Credential.Value, c.ClientIP(), capability, "", true)
	// phone_verified 供插件决定是否引导客户完成手机号认证。
	phoneVerified, err := h.auth.PhoneVerified(c.Request.Context(), membership.CustomerUUID)
	if err != nil {
		respondMembershipError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"access_token":   pair.AccessToken,
		"refresh_token":  pair.RefreshToken,
		"expires_in":     pair.ExpiresIn,
		"phone_verified": phoneVerified,
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
	tenantUUID, customerUUID, _, ok := h.currentCustomer(c, capabilityID)
	if !ok {
		return
	}
	item, err := h.memberships.ResolveCurrent(c.Request.Context(), tenantUUID, customerUUID)
	if err != nil {
		respondMembershipError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"item": item})
}

// currentCustomer 校验插件 STS（含能力授权）与当前客户 token，返回租户、客户与插件；失败时已写出响应。
// 租户、客户只来自凭证，请求里携带的租户 / 客户标识一律拒绝。
func (h *handler) currentCustomer(c *gin.Context, capabilityID string) (string, string, string, bool) {
	if c.Request.URL.RawQuery != "" || c.GetHeader("X-Tenant-UUID") != "" || c.GetHeader("X-Customer-UUID") != "" || c.GetHeader("X-Membership-UUID") != "" {
		respondError(c, http.StatusBadRequest, "CUSTOMER_UNAUTHORIZED", customersvc.ErrCustomerUnauthorized)
		return "", "", "", false
	}
	claims := reqctx.GetClaims(c.Request.Context())
	if claims == nil || !strings.EqualFold(claims.Issuer, "powerx-sts") || !hasAudience(claims.Audience, "powerx:api") || strings.TrimSpace(claims.PluginID) == "" {
		respondError(c, http.StatusUnauthorized, "CUSTOMER_UNAUTHORIZED", customersvc.ErrCustomerUnauthorized)
		return "", "", "", false
	}
	tenantUUID, err := reqctx.CanonicalTenantUUID(claims.TenantUUID)
	if err != nil {
		respondError(c, http.StatusUnauthorized, "CUSTOMER_UNAUTHORIZED", customersvc.ErrCustomerUnauthorized)
		return "", "", "", false
	}
	if err := h.memberships.AuthorizeDelegatedActorForCapability(c.Request.Context(), tenantUUID, claims.PluginID, capabilityID); err != nil {
		respondMembershipError(c, err)
		return "", "", "", false
	}
	raw := strings.TrimSpace(c.GetHeader(customerAuthorizationHeader))
	if !strings.HasPrefix(strings.ToLower(raw), "bearer ") {
		respondError(c, http.StatusUnauthorized, "CUSTOMER_UNAUTHORIZED", customersvc.ErrCustomerUnauthorized)
		return "", "", "", false
	}
	customerClaims, err := h.tokens.Validate(c.Request.Context(), strings.TrimSpace(raw[7:]))
	if err != nil || customerClaims == nil {
		respondError(c, http.StatusUnauthorized, "CUSTOMER_UNAUTHORIZED", customersvc.ErrCustomerUnauthorized)
		return "", "", "", false
	}
	customerUUID, customerErr := uuid.Parse(strings.TrimSpace(customerClaims.CustomerUUID))
	if customerErr != nil || customerClaims.TenantUUID != tenantUUID {
		respondError(c, http.StatusUnauthorized, "CUSTOMER_UNAUTHORIZED", customersvc.ErrCustomerUnauthorized)
		return "", "", "", false
	}
	return tenantUUID, customerUUID.String(), claims.PluginID, true
}

type phoneVerifyRequest struct {
	Channel    string `json:"channel" binding:"required"`
	AppID      string `json:"app_id"`
	Credential struct {
		Type  string `json:"type" binding:"required"`
		Value string `json:"value" binding:"required"`
	} `json:"credential"`
}

// verifyPhone 为当前客户认证手机号（小程序 getPhoneNumber 的一次性 code）。
func (h *handler) verifyPhone(c *gin.Context) {
	var req phoneVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "CUSTOMER_INVALID_ARGUMENT", err)
		return
	}
	tenantUUID, customerUUID, pluginID, ok := h.currentCustomer(c, customersvc.CustomerPhoneVerifyCapabilityID)
	if !ok {
		return
	}
	want, isMiniApp := customersvc.MiniAppPhoneCredentialType(req.Channel)
	if !isMiniApp || strings.TrimSpace(req.Credential.Type) != want || strings.TrimSpace(req.AppID) == "" {
		respondError(c, http.StatusBadRequest, "CUSTOMER_INVALID_ARGUMENT", customersvc.ErrCustomerPhoneCodeInvalid)
		return
	}
	channel := strings.TrimSpace(req.Channel) + "_phone"
	if err := h.auth.AllowAttempt(c.Request.Context(), tenantUUID, pluginID, channel, customerUUID, c.ClientIP()); err != nil {
		h.auth.RecordAttempt(c.Request.Context(), tenantUUID, pluginID, channel, customerUUID, c.ClientIP(), customersvc.CustomerPhoneVerifyCapabilityID, "CUSTOMER_FORBIDDEN", false)
		respondMembershipError(c, err)
		return
	}
	out, err := h.auth.VerifyMiniAppPhone(c.Request.Context(), tenantUUID, customerUUID, req.Channel, req.AppID, req.Credential.Value)
	if err != nil {
		h.auth.RecordAttempt(c.Request.Context(), tenantUUID, pluginID, channel, customerUUID, c.ClientIP(), customersvc.CustomerPhoneVerifyCapabilityID, "CUSTOMER_PHONE_VERIFY_FAILED", false)
		respondAuthError(c, err)
		return
	}
	h.auth.RecordAttempt(c.Request.Context(), tenantUUID, pluginID, channel, customerUUID, c.ClientIP(), customersvc.CustomerPhoneVerifyCapabilityID, "", true)
	dto.ResponseSuccess(c, gin.H{"phone_masked": out.PhoneMasked, "verified_at": out.VerifiedAt, "phone_verified": true})
}

func isMiniAppCredential(channel, credentialType string) bool {
	want, ok := customersvc.MiniAppCredentialType(channel)
	return ok && strings.TrimSpace(credentialType) == want
}

func respondAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, customersvc.ErrCustomerClientAppNotFound):
		respondError(c, http.StatusBadRequest, "CUSTOMER_CLIENT_APP_NOT_FOUND", err)
	case errors.Is(err, customersvc.ErrCustomerClientAppForbidden):
		respondError(c, http.StatusForbidden, "CUSTOMER_CLIENT_APP_FORBIDDEN", err)
	case errors.Is(err, customersvc.ErrCustomerClientPlatformUnsupported):
		respondError(c, http.StatusBadRequest, "CUSTOMER_CLIENT_PLATFORM_UNSUPPORTED", err)
	case errors.Is(err, customersvc.ErrCustomerPhoneCodeInvalid):
		respondError(c, http.StatusUnauthorized, "CUSTOMER_PHONE_CODE_INVALID", err)
	case errors.Is(err, customersvc.ErrCustomerPhoneConflict):
		respondError(c, http.StatusConflict, "CUSTOMER_PHONE_CONFLICT", err)
	case errors.Is(err, customersvc.ErrCustomerClientAppNotConfigured):
		respondError(c, http.StatusServiceUnavailable, "CUSTOMER_CLIENT_APP_NOT_CONFIGURED", err)
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
