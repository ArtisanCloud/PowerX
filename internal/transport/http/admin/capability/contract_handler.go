package capability

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	validator "github.com/ArtisanCloud/PowerX/internal/contract/capability"
	svc "github.com/ArtisanCloud/PowerX/internal/service/capability"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ContractHandler 处理能力契约的 HTTP 请求。
type ContractHandler struct {
	svc *svc.ContractService
}

// NewContractHandler 创建处理器实例。
func NewContractHandler(deps *shared.Deps) *ContractHandler {
	validator := validator.NewValidator(validator.ValidatorOptions{})
	service := svc.NewContractService(deps.DB, validator, deps.AuditSvc)
	return &ContractHandler{svc: service}
}

// CreateContract 创建契约草稿。
func (h *ContractHandler) CreateContract(c *gin.Context) {
	var req contractPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBadRequest(c, "invalid_payload", err.Error())
		return
	}
	tenantID := tenantIDFromRequest(c, req.TenantID)
	input := req.toUpsertInput(tenantID)
	contract, issues, err := h.svc.UpsertDraft(c.Request.Context(), input)
	if err != nil {
		if errorsIsValidation(err) {
			writeValidationError(c, issues)
			return
		}
		writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toContractResponse(contract))
}

// UpdateContract 更新契约草稿。
func (h *ContractHandler) UpdateContract(c *gin.Context) {
	var req contractPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBadRequest(c, "invalid_payload", err.Error())
		return
	}
	capabilityKey := c.Param("capabilityKey")
	version := c.Param("version")
	tenantID := tenantIDFromRequest(c, req.TenantID)
	if capabilityKey == "" || version == "" {
		writeBadRequest(c, "missing_path", "capability key 或 version 缺失")
		return
	}
	req.CapabilityKey = capabilityKey
	req.Version = version
	input := req.toUpsertInput(tenantID)
	contract, issues, err := h.svc.UpsertDraft(c.Request.Context(), input)
	if err != nil {
		if errorsIsValidation(err) {
			writeValidationError(c, issues)
			return
		}
		writeInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, toContractResponse(contract))
}

// GetContract 获取单个契约。
func (h *ContractHandler) GetContract(c *gin.Context) {
	capabilityKey := c.Param("capabilityKey")
	version := c.Param("version")
	tenantID := tenantIDFromQuery(c.Query("tenant_id"))
	if capabilityKey == "" || version == "" {
		writeBadRequest(c, "missing_path", "capability key 或 version 缺失")
		return
	}
	contract, err := h.svc.GetContract(c.Request.Context(), tenantID, capabilityKey, version)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, toContractResponse(contract))
}

// ListContracts 列出契约。
func (h *ContractHandler) ListContracts(c *gin.Context) {
	tenantID := tenantIDFromQuery(c.Query("tenant_id"))
	keyword := c.Query("capability_key")
	limit := parseIntDefault(c.Query("page_size"), 20)
	page := parseIntDefault(c.Query("page"), 1)
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit
	items, total, err := h.svc.ListContracts(c.Request.Context(), tenantID, keyword, limit, offset)
	if err != nil {
		writeInternalError(c, err)
		return
	}
	views := make([]ContractResponse, 0, len(items))
	for _, item := range items {
		views = append(views, toContractResponse(item))
	}
	c.JSON(http.StatusOK, gin.H{
		"items": views,
		"total": total,
	})
}

// PublishContract 发布契约。
func (h *ContractHandler) PublishContract(c *gin.Context) {
	capabilityKey := c.Param("capabilityKey")
	version := c.Param("version")
	if capabilityKey == "" || version == "" {
		writeBadRequest(c, "missing_path", "capability key 或 version 缺失")
		return
	}
	var req publishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBadRequest(c, "invalid_payload", err.Error())
		return
	}
	if req.EffectiveAt == nil {
		writeBadRequest(c, "missing_effective_at", "effective_at 必填，需使用 RFC3339 时间格式")
		return
	}
	tenantID := tenantIDFromQuery(c.Query("tenant_id"))
	input := &svc.PublishInput{
		TenantID:      tenantID,
		CapabilityKey: capabilityKey,
		Version:       version,
		EffectiveAt:   *req.EffectiveAt,
		Notes:         req.Notes,
	}
	contract, issues, err := h.svc.PublishContract(c.Request.Context(), input)
	if err != nil {
		if errorsIsValidation(err) {
			writeValidationError(c, issues)
			return
		}
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, toContractResponse(contract))
}

// DeprecateContract 标记契约为废弃。
func (h *ContractHandler) DeprecateContract(c *gin.Context) {
	capabilityKey := c.Param("capabilityKey")
	version := c.Param("version")
	if capabilityKey == "" || version == "" {
		writeBadRequest(c, "missing_path", "capability key 或 version 缺失")
		return
	}
	var req deprecateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBadRequest(c, "invalid_payload", err.Error())
		return
	}
	if req.DeprecatedAt == nil {
		writeBadRequest(c, "missing_deprecated_at", "deprecated_at 必填，需使用 RFC3339 时间格式")
		return
	}
	tenantID := tenantIDFromQuery(c.Query("tenant_id"))
	input := &svc.DeprecateInput{
		TenantID:              tenantID,
		CapabilityKey:         capabilityKey,
		Version:               version,
		DeprecatedAt:          *req.DeprecatedAt,
		ReplacementCapability: req.ReplacementCapability,
		AdvisoryMessage:       req.AdvisoryMessage,
	}
	contract, err := h.svc.DeprecateContract(c.Request.Context(), input)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, toContractResponse(contract))
}

// ---------- 请求/响应结构 ----------

type contractPayload struct {
	TenantID             *uint64                      `json:"tenant_id,omitempty"`
	CapabilityKey        string                       `json:"capability_key"`
	Version              string                       `json:"version"`
	ProviderID           string                       `json:"provider_id"`
	DisplayName          string                       `json:"display_name"`
	Description          string                       `json:"description"`
	SecurityScope        string                       `json:"security_scope"`
	ToolGrantRequired    bool                         `json:"tool_grant_required"`
	ObservabilityConfig  map[string]interface{}       `json:"observability_config"`
	IOSchemas            []ioSchemaPayload            `json:"io_schemas"`
	TransportPreferences []transportPreferencePayload `json:"transport_preferences"`
	TransportProfiles    []transportProfilePayload    `json:"transport_profiles"`
	ErrorTaxonomy        []errorTaxonomyPayload       `json:"error_taxonomy"`
}

type ioSchemaPayload struct {
	Direction       string                 `json:"direction"`
	Format          string                 `json:"format"`
	SchemaURI       string                 `json:"schema_uri"`
	SchemaHash      string                 `json:"schema_hash"`
	ValidationRules map[string]interface{} `json:"validation_rules"`
}

type transportPreferencePayload struct {
	Transport string `json:"transport"`
	Mode      string `json:"mode"`
}

type transportProfilePayload struct {
	Transport        string                 `json:"transport"`
	Mode             string                 `json:"mode"`
	TimeoutMillis    int                    `json:"timeout_ms"`
	Streaming        bool                   `json:"streaming"`
	Retry            map[string]interface{} `json:"retry"`
	QoS              map[string]interface{} `json:"qos"`
	EndpointSelector map[string]interface{} `json:"endpoint_selector"`
}

type errorTaxonomyPayload struct {
	Namespace string `json:"namespace"`
	Category  string `json:"category"`
	Code      string `json:"code"`
	Severity  string `json:"severity"`
	Stage     string `json:"stage"`
}

type publishRequest struct {
	EffectiveAt *time.Time `json:"effective_at"`
	Notes       string     `json:"notes"`
}

type deprecateRequest struct {
	ReplacementCapability string     `json:"replacement_capability"`
	DeprecatedAt          *time.Time `json:"deprecated_at"`
	AdvisoryMessage       string     `json:"advisory_message"`
}

type ContractResponse struct {
	ID                    uint64                          `json:"id"`
	ContractUUID          string                          `json:"contract_uuid"`
	TenantID              uint64                          `json:"tenant_id"`
	CapabilityKey         string                          `json:"capability_key"`
	Version               string                          `json:"version"`
	ProviderID            string                          `json:"provider_id"`
	DisplayName           string                          `json:"display_name"`
	Description           string                          `json:"description"`
	LifecycleState        string                          `json:"lifecycle_state"`
	SecurityScope         string                          `json:"security_scope"`
	ToolGrantRequired     bool                            `json:"tool_grant_required"`
	ObservabilityConfig   map[string]interface{}          `json:"observability_config"`
	IOSchemas             []validator.IOSchemaDescriptor  `json:"io_schemas"`
	TransportPreferences  []validator.TransportPreference `json:"transport_preferences"`
	TransportProfiles     []validator.TransportProfile    `json:"transport_profiles"`
	ErrorTaxonomy         []validator.ErrorTaxonomyEntry  `json:"error_taxonomy"`
	EffectiveAt           *time.Time                      `json:"effective_at"`
	DeprecatedAt          *time.Time                      `json:"deprecated_at"`
	ReplacementCapability string                          `json:"replacement_capability"`
	CreatedAt             time.Time                       `json:"created_at"`
	UpdatedAt             time.Time                       `json:"updated_at"`
}

func (p *contractPayload) toUpsertInput(tenantID uint64) *svc.ContractUpsertInput {
	return &svc.ContractUpsertInput{
		TenantID:             tenantID,
		CapabilityKey:        strings.TrimSpace(p.CapabilityKey),
		Version:              strings.TrimSpace(p.Version),
		ProviderID:           p.ProviderID,
		DisplayName:          p.DisplayName,
		Description:          p.Description,
		SecurityScope:        p.SecurityScope,
		ToolGrantRequired:    p.ToolGrantRequired,
		ObservabilityConfig:  p.ObservabilityConfig,
		IOSchemas:            toIOSchemaDescriptors(p.IOSchemas),
		TransportPreferences: toTransportPreferences(p.TransportPreferences),
		TransportProfiles:    toTransportProfiles(p.TransportProfiles),
		ErrorTaxonomy:        toErrorTaxonomies(p.ErrorTaxonomy),
	}
}

func toContractResponse(model *svc.Contract) ContractResponse {
	if model == nil {
		return ContractResponse{}
	}
	return ContractResponse{
		ID:                    model.ID,
		ContractUUID:          model.ContractUUID,
		TenantID:              model.TenantID,
		CapabilityKey:         model.CapabilityKey,
		Version:               model.Version,
		ProviderID:            model.ProviderID,
		DisplayName:           model.DisplayName,
		Description:           model.Description,
		LifecycleState:        model.LifecycleState,
		SecurityScope:         model.SecurityScope,
		ToolGrantRequired:     model.ToolGrantRequired,
		ObservabilityConfig:   model.ObservabilityConfig,
		IOSchemas:             model.IOSchemas,
		TransportPreferences:  model.TransportPreferences,
		TransportProfiles:     model.TransportProfiles,
		ErrorTaxonomy:         model.ErrorTaxonomy,
		EffectiveAt:           model.EffectiveAt,
		DeprecatedAt:          model.DeprecatedAt,
		ReplacementCapability: model.ReplacementCapability,
		CreatedAt:             model.CreatedAt,
		UpdatedAt:             model.UpdatedAt,
	}
}

func toIOSchemaDescriptors(items []ioSchemaPayload) []validator.IOSchemaDescriptor {
	result := make([]validator.IOSchemaDescriptor, 0, len(items))
	for _, item := range items {
		result = append(result, validator.IOSchemaDescriptor{
			Direction:       item.Direction,
			Format:          item.Format,
			SchemaURI:       item.SchemaURI,
			SchemaHash:      item.SchemaHash,
			ValidationRules: item.ValidationRules,
		})
	}
	return result
}

func toTransportPreferences(items []transportPreferencePayload) []validator.TransportPreference {
	result := make([]validator.TransportPreference, 0, len(items))
	for _, item := range items {
		result = append(result, validator.TransportPreference{Transport: item.Transport, Mode: item.Mode})
	}
	return result
}

func toTransportProfiles(items []transportProfilePayload) []validator.TransportProfile {
	result := make([]validator.TransportProfile, 0, len(items))
	for _, item := range items {
		result = append(result, validator.TransportProfile{
			Transport:        item.Transport,
			Mode:             item.Mode,
			TimeoutMillis:    item.TimeoutMillis,
			Streaming:        item.Streaming,
			Retry:            item.Retry,
			QoS:              item.QoS,
			EndpointSelector: item.EndpointSelector,
		})
	}
	return result
}

func toErrorTaxonomies(items []errorTaxonomyPayload) []validator.ErrorTaxonomyEntry {
	result := make([]validator.ErrorTaxonomyEntry, 0, len(items))
	for _, item := range items {
		result = append(result, validator.ErrorTaxonomyEntry{
			Namespace: item.Namespace,
			Category:  item.Category,
			Code:      item.Code,
			Severity:  item.Severity,
			Stage:     item.Stage,
		})
	}
	return result
}

// ---------- 辅助函数 ----------

func tenantIDFromRequest(c *gin.Context, payloadValue *uint64) uint64 {
	if payloadValue != nil {
		return *payloadValue
	}
	return tenantIDFromQuery(c.Query("tenant_id"))
}

func tenantIDFromQuery(val string) uint64 {
	if val == "" {
		return 0
	}
	id, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func parseIntDefault(val string, def int) int {
	if val == "" {
		return def
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return i
}

func writeValidationError(c *gin.Context, issues []validator.ValidationIssue) {
	c.JSON(http.StatusBadRequest, gin.H{
		"error":  "validation_failed",
		"issues": issues,
	})
}

func writeBadRequest(c *gin.Context, code, message string) {
	c.JSON(http.StatusBadRequest, gin.H{
		"error":   code,
		"message": message,
	})
}

func writeServiceError(c *gin.Context, err error) {
	if errorsIsNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":   "not_found",
			"message": err.Error(),
		})
		return
	}
	writeInternalError(c, err)
}

func writeInternalError(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":   "internal_error",
		"message": err.Error(),
	})
}

func errorsIsValidation(err error) bool {
	return errors.Is(err, svc.ErrValidation)
}

func errorsIsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
