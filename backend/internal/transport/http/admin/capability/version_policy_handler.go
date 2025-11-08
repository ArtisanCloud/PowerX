package capability

import (
	"errors"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	svc "github.com/ArtisanCloud/PowerX/internal/service/capability"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// VersionPolicyHandler 处理版本策略相关接口。
type VersionPolicyHandler struct {
	svc *svc.VersionPolicyService
}

// NewVersionPolicyHandler 创建处理器。
func NewVersionPolicyHandler(deps *shared.Deps) *VersionPolicyHandler {
	return &VersionPolicyHandler{
		svc: svc.NewVersionPolicyService(deps.DB, deps.AuditSvc),
	}
}

// GetVersionPolicy 获取指定能力的版本策略。
func (h *VersionPolicyHandler) GetVersionPolicy(c *gin.Context) {
	capabilityKey := c.Param("capabilityKey")
	if capabilityKey == "" {
		writeBadRequest(c, "missing_path", "capability key 缺失")
		return
	}
	tenantID := tenantIDFromQuery(c.Query("tenant_id"))
	policy, err := h.svc.GetVersionPolicy(c.Request.Context(), tenantID, capabilityKey)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			dto.ResponseSuccess(c, gin.H{"policy": nil})
			return
		}
		writeInternalError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"policy": toPolicyResponse(policy)})
}

// UpsertVersionPolicy 创建或更新策略。
func (h *VersionPolicyHandler) UpsertVersionPolicy(c *gin.Context) {
	capabilityKey := c.Param("capabilityKey")
	if capabilityKey == "" {
		writeBadRequest(c, "missing_path", "capability key 缺失")
		return
	}
	var req versionPolicyPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBadRequest(c, "invalid_payload", err.Error())
		return
	}
	tenantID := tenantIDFromRequest(c, req.TenantID)
	input := &svc.VersionPolicyUpsertInput{
		TenantID:            tenantID,
		CapabilityKey:       capabilityKey,
		DefaultStrategy:     req.DefaultStrategy,
		AllowedVersions:     toServiceVersionRules(req.AllowedVersions),
		CompatibilityMatrix: req.CompatibilityMatrix,
		DeprecationPolicy:   req.DeprecationPolicy,
		AuditConfig:         req.AuditConfig,
	}
	policy, err := h.svc.UpsertVersionPolicy(c.Request.Context(), input)
	if err != nil {
		if errors.Is(err, svc.ErrPolicyValidation) {
			writeBadRequest(c, "validation_failed", err.Error())
			return
		}
		writeInternalError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"policy": toPolicyResponse(policy)})
}

// ---------- 请求/响应模型 ----------

type versionPolicyPayload struct {
	TenantID            *uint64                `json:"tenant_id"`
	DefaultStrategy     string                 `json:"default_strategy"`
	AllowedVersions     []versionRulePayload   `json:"allowed_versions"`
	CompatibilityMatrix map[string]interface{} `json:"compatibility_matrix"`
	DeprecationPolicy   map[string]interface{} `json:"deprecation_policy"`
	AuditConfig         map[string]interface{} `json:"audit_config"`
}

type versionRulePayload struct {
	Version        string   `json:"version"`
	CompatibleWith []string `json:"compatible_with"`
	Status         string   `json:"status"`
}

type versionPolicyResponse struct {
	DefaultStrategy     string                 `json:"default_strategy"`
	AllowedVersions     []versionRulePayload   `json:"allowed_versions"`
	CompatibilityMatrix map[string]interface{} `json:"compatibility_matrix"`
	DeprecationPolicy   map[string]interface{} `json:"deprecation_policy"`
	AuditConfig         map[string]interface{} `json:"audit_config"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

func toPolicyResponse(policy *svc.VersionPolicy) versionPolicyResponse {
	if policy == nil {
		return versionPolicyResponse{}
	}
	return versionPolicyResponse{
		DefaultStrategy:     policy.DefaultStrategy,
		AllowedVersions:     fromServiceVersionRules(policy.AllowedVersions),
		CompatibilityMatrix: policy.CompatibilityMatrix,
		DeprecationPolicy:   policy.DeprecationPolicy,
		AuditConfig:         policy.AuditConfig,
		UpdatedAt:           policy.UpdatedAt,
	}
}

func toServiceVersionRules(items []versionRulePayload) []svc.VersionRule {
	result := make([]svc.VersionRule, 0, len(items))
	for _, item := range items {
		status := item.Status
		if status == "" {
			status = "active"
		}
		result = append(result, svc.VersionRule{
			Version:        strings.TrimSpace(item.Version),
			CompatibleWith: item.CompatibleWith,
			Status:         status,
		})
	}
	return result
}

func fromServiceVersionRules(items []svc.VersionRule) []versionRulePayload {
	result := make([]versionRulePayload, 0, len(items))
	for _, item := range items {
		result = append(result, versionRulePayload{
			Version:        item.Version,
			CompatibleWith: item.CompatibleWith,
			Status:         item.Status,
		})
	}
	return result
}
