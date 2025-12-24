package capability_registrydto

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	capabilitycatalog "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	modelregistry "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"gorm.io/datatypes"
)

// CapabilityRecordDTO mirrors the HTTP schema returned by catalog endpoints.
type CapabilityRecordDTO struct {
	CapabilityID      string                `json:"capability_id"`
	PluginID          string                `json:"plugin_id"`
	PluginVersion     string                `json:"plugin_version"`
	Title             string                `json:"title"`
	Description       string                `json:"description,omitempty"`
	Source            string                `json:"source"`
	Categories        []string              `json:"categories,omitempty"`
	Intents           []string              `json:"intents,omitempty"`
	ToolScope         []string              `json:"tool_scope,omitempty"`
	Policy            *CapabilityPolicyDTO  `json:"policy,omitempty"`
	Protocols         []ProtocolBindingDTO  `json:"protocols"`
	WorkflowTemplates []WorkflowTemplateDTO `json:"workflow_templates,omitempty"`
	CompositeGraphs   interface{}           `json:"composite_graphs,omitempty"`
	Annotations       interface{}           `json:"annotations,omitempty"`
	CapabilitiesHash  string                `json:"capabilities_hash"`
	ProtocolHash      string                `json:"protocol_hash"`
	Status            string                `json:"status"`
	PublishedAt       *string               `json:"published_at,omitempty"`
}

// CapabilityPolicyDTO describes policy preferences per capability.
type CapabilityPolicyDTO struct {
	Prefer             string   `json:"prefer,omitempty"`
	Fallback           []string `json:"fallback,omitempty"`
	RollbackCapability string   `json:"rollback_capability_id,omitempty"`
}

// ProtocolBindingDTO exposes protocol endpoints.
type ProtocolBindingDTO struct {
	Channel       string  `json:"channel"`
	Endpoint      string  `json:"endpoint,omitempty"`
	SchemaRef     string  `json:"schema_ref,omitempty"`
	Method        string  `json:"method,omitempty"`
	RPC           string  `json:"rpc,omitempty"`
	ToolRef       string  `json:"tool_ref,omitempty"`
	AuthType      string  `json:"auth_type,omitempty"`
	HealthState   string  `json:"health_state,omitempty"`
	LastCheckedAt *string `json:"last_checked_at,omitempty"`
}

// WorkflowTemplateDTO mirrors workflow template refs stored in registry.
type WorkflowTemplateDTO struct {
	TemplateID               string      `json:"template_id"`
	Name                     string      `json:"name"`
	Description              string      `json:"description,omitempty"`
	Steps                    interface{} `json:"steps,omitempty"`
	ParamsSchema             interface{} `json:"params_schema,omitempty"`
	ProtocolRequirements     interface{} `json:"protocol_requirements,omitempty"`
	RequiresManualUpgrade    bool        `json:"requires_manual_upgrade"`
	CapabilitiesHashSnapshot string      `json:"capabilities_hash_snapshot"`
	TemplateHash             string      `json:"template_hash"`
	LastSyncedAt             *string     `json:"last_synced_at,omitempty"`
}

// WorkflowCatalogTemplateDTO 为 Builder UI 提供模板列表。
type WorkflowCatalogTemplateDTO struct {
	TemplateID            string                       `json:"template_id"`
	CapabilityID          string                       `json:"capability_id"`
	CapabilityTitle       string                       `json:"capability_title,omitempty"`
	PluginID              string                       `json:"plugin_id,omitempty"`
	Name                  string                       `json:"name"`
	Description           string                       `json:"description,omitempty"`
	Steps                 interface{}                  `json:"steps,omitempty"`
	ParamsSchema          interface{}                  `json:"params_schema,omitempty"`
	ProtocolRequirements  interface{}                  `json:"protocol_requirements,omitempty"`
	CapabilitiesHash      string                       `json:"capabilities_hash"`
	TemplateHash          string                       `json:"template_hash"`
	RequiresManualUpgrade bool                         `json:"requires_manual_upgrade"`
	Approved              *WorkflowTemplateApprovalDTO `json:"approved,omitempty"`
	NeedsUpgrade          bool                         `json:"needs_upgrade"`
	LastSyncedAt          *string                      `json:"last_synced_at,omitempty"`
}

// CapabilitySyncJobDTO mirrors capability sync job responses.
type CapabilitySyncJobDTO struct {
	JobID         string  `json:"job_id"`
	CapabilityID  string  `json:"capability_id,omitempty"`
	PluginID      string  `json:"plugin_id"`
	PluginVersion string  `json:"plugin_version"`
	Status        string  `json:"status"`
	HashBefore    string  `json:"hash_before,omitempty"`
	HashAfter     string  `json:"hash_after,omitempty"`
	ErrorSummary  string  `json:"error_summary,omitempty"`
	StartedAt     *string `json:"started_at,omitempty"`
	FinishedAt    *string `json:"finished_at,omitempty"`
}

// CapabilityViewToDTO converts a registry view to DTO.
func CapabilityViewToDTO(view capabilitycatalog.CapabilityRecordView, includeWorkflows bool) CapabilityRecordDTO {
	record := view.Record
	dto := CapabilityRecordDTO{
		CapabilityID:     record.CapabilityID,
		PluginID:         record.PluginID,
		PluginVersion:    record.PluginVersion,
		Title:            record.Title,
		Description:      record.Description,
		Source:           capabilitycatalog.CapabilitySource(record),
		Categories:       decodeStringArray(record.Categories),
		Intents:          decodeStringArray(record.Intents),
		ToolScope:        decodeStringArray(record.ToolScope),
		Policy:           decodePolicy(record.Policy),
		Protocols:        decodeProtocols(record.Protocols),
		CompositeGraphs:  decodeJSONRaw(record.CompositeGraphs),
		Annotations:      decodeJSONRaw(record.Annotations),
		CapabilitiesHash: record.CapabilitiesHash,
		ProtocolHash:     record.ProtocolHash,
		Status:           record.Status,
		PublishedAt:      formatTime(record.PublishedAt),
	}
	if includeWorkflows {
		dto.WorkflowTemplates = make([]WorkflowTemplateDTO, 0, len(view.WorkflowTemplates))
		for _, tpl := range view.WorkflowTemplates {
			dto.WorkflowTemplates = append(dto.WorkflowTemplates, WorkflowTemplateToDTO(tpl))
		}
	}
	return dto
}

// WorkflowTemplateApprovalDTO 描述模板审批记录。
type WorkflowTemplateApprovalDTO struct {
	TemplateID       string  `json:"template_id"`
	CapabilityID     string  `json:"capability_id"`
	CapabilitiesHash string  `json:"capabilities_hash"`
	Reason           string  `json:"reason,omitempty"`
	ApprovedBy       string  `json:"approved_by,omitempty"`
	ApprovedAt       *string `json:"approved_at,omitempty"`
}

// WorkflowTemplateApprovalToDTO converts approval model to DTO.
func WorkflowTemplateApprovalToDTO(model *modelregistry.WorkflowTemplateApproval) WorkflowTemplateApprovalDTO {
	if model == nil {
		return WorkflowTemplateApprovalDTO{}
	}
	return WorkflowTemplateApprovalDTO{
		TemplateID:       model.TemplateID,
		CapabilityID:     model.CapabilityID,
		CapabilitiesHash: model.CapabilitiesHash,
		Reason:           model.Reason,
		ApprovedBy:       model.ApprovedBy,
		ApprovedAt:       formatTime(&model.ApprovedAt),
	}
}

// WorkflowTemplateToDTO converts a workflow template record to DTO.
func WorkflowTemplateToDTO(model modelregistry.WorkflowTemplateRef) WorkflowTemplateDTO {
	return WorkflowTemplateDTO{
		TemplateID:               model.TemplateID,
		Name:                     model.Name,
		Description:              model.Description,
		Steps:                    decodeJSONRaw(model.Steps),
		ParamsSchema:             decodeJSONRaw(model.ParamsSchema),
		ProtocolRequirements:     decodeJSONRaw(model.ProtocolRequirements),
		RequiresManualUpgrade:    model.RequiresManualUpgrade,
		CapabilitiesHashSnapshot: model.CapabilitiesHash,
		TemplateHash:             model.TemplateHash,
		LastSyncedAt:             formatTime(model.LastSyncedAt),
	}
}

// WorkflowCatalogTemplateToDTO converts catalog snapshot items to DTO.
func WorkflowCatalogTemplateToDTO(model capabilitycatalog.WorkflowCatalogTemplate, approval *modelregistry.WorkflowTemplateApproval) WorkflowCatalogTemplateDTO {
	dto := WorkflowCatalogTemplateDTO{
		TemplateID:            model.TemplateID,
		CapabilityID:          model.CapabilityID,
		CapabilityTitle:       model.CapabilityTitle,
		PluginID:              model.PluginID,
		Name:                  model.Name,
		Description:           model.Description,
		Steps:                 decodeRawMessage(model.Steps),
		ParamsSchema:          decodeRawMessage(model.ParamsSchema),
		ProtocolRequirements:  decodeRawMessage(model.ProtocolRequirements),
		CapabilitiesHash:      model.CapabilitiesHash,
		TemplateHash:          model.TemplateHash,
		RequiresManualUpgrade: model.RequiresManualUpgrade,
		LastSyncedAt:          formatTime(model.LastSyncedAt),
	}
	if model.RequiresManualUpgrade {
		dto.NeedsUpgrade = true
	}
	if approval != nil {
		approvalDTO := WorkflowTemplateApprovalToDTO(approval)
		dto.Approved = &approvalDTO
		if strings.EqualFold(strings.TrimSpace(approval.CapabilitiesHash), strings.TrimSpace(model.CapabilitiesHash)) {
			dto.NeedsUpgrade = false
		}
	}
	return dto
}

func decodeRawMessage(raw json.RawMessage) interface{} {
	if len(raw) == 0 {
		return nil
	}
	var out interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return json.RawMessage(raw)
	}
	return out
}

// SyncJobToDTO converts a sync job model to DTO.
func SyncJobToDTO(job modelregistry.CapabilitySyncJob) CapabilitySyncJobDTO {
	return CapabilitySyncJobDTO{
		JobID:         job.UUID.String(),
		CapabilityID:  job.CapabilityID,
		PluginID:      job.PluginID,
		PluginVersion: job.PluginVersion,
		Status:        job.Status,
		HashBefore:    job.HashBefore,
		HashAfter:     job.HashAfter,
		ErrorSummary:  job.ErrorSummary,
		StartedAt:     formatTime(&job.StartedAt),
		FinishedAt:    formatTime(job.FinishedAt),
	}
}

func decodeProtocols(raw []byte) []ProtocolBindingDTO {
	if len(raw) == 0 {
		return nil
	}
	var bindings []modelregistry.ProtocolBinding
	if err := json.Unmarshal(raw, &bindings); err != nil {
		return nil
	}
	dtos := make([]ProtocolBindingDTO, 0, len(bindings))
	for _, binding := range bindings {
		dto := ProtocolBindingDTO{
			Channel:     binding.Channel,
			Endpoint:    binding.Endpoint,
			SchemaRef:   binding.SchemaRef,
			Method:      binding.Method,
			RPC:         binding.RPC,
			ToolRef:     binding.ToolRef,
			AuthType:    binding.AuthType,
			HealthState: binding.HealthState,
		}
		if binding.LastChecked != "" {
			dto.LastCheckedAt = stringPtr(binding.LastChecked)
		}
		dtos = append(dtos, dto)
	}
	return dtos
}

func decodePolicy(raw []byte) *CapabilityPolicyDTO {
	if len(raw) == 0 {
		return nil
	}
	var dto CapabilityPolicyDTO
	if err := json.Unmarshal(raw, &dto); err != nil {
		return nil
	}
	return &dto
}

func decodeStringArray(raw datatypes.JSON) []string {
	if len(raw) == 0 {
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	return values
}

func decodeJSONRaw(raw datatypes.JSON) interface{} {
	if len(raw) == 0 {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	if t.IsZero() {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	v := value
	return &v
}

// PlatformCapabilityModuleDTO describes grouped CoreX platform capabilities for Admin UI.
type PlatformCapabilityModuleDTO struct {
	Module           string                  `json:"module"`
	DisplayName      string                  `json:"display_name,omitempty"`
	Description      string                  `json:"description,omitempty"`
	CapabilityCount  int                     `json:"capability_count"`
	ProtocolChannels []string                `json:"protocol_channels,omitempty"`
	Capabilities     []PlatformCapabilityDTO `json:"capabilities"`
}

// PlatformCapabilityDTO exposes a single CoreX platform capability entry.
type PlatformCapabilityDTO struct {
	CapabilityID      string                          `json:"capability_id"`
	Title             string                          `json:"title"`
	Description       string                          `json:"description,omitempty"`
	Module            string                          `json:"module"`
	Source            string                          `json:"source"`
	PluginID          string                          `json:"plugin_id"`
	PluginVersion     string                          `json:"plugin_version"`
	Docs              []string                        `json:"docs,omitempty"`
	CapabilitiesHash  string                          `json:"capabilities_hash"`
	PreferredProtocol string                          `json:"preferred_protocol,omitempty"`
	Protocols         []ProtocolBindingDTO            `json:"protocols"`
	DebugExamples     PlatformCapabilityDebugExamples `json:"debug_examples"`
}

// PlatformCapabilityDebugExamples contains helper snippets for host/skeleton plugins.
type PlatformCapabilityDebugExamples struct {
	TenantInvocationCurl    string                 `json:"tenant_invocation_curl"`
	TenantInvocationPayload map[string]interface{} `json:"tenant_invocation_payload,omitempty"`
}

// PlatformCapabilityToDTO converts a registry record into platform DTO.
func PlatformCapabilityToDTO(record *modelregistry.CapabilityRecord) PlatformCapabilityDTO {
	if record == nil {
		return PlatformCapabilityDTO{}
	}
	moduleMetadata := parsePlatformAnnotations(record)
	protocols := decodeProtocols(record.Protocols)
	preferred := ""
	if policy := decodePolicy(record.Policy); policy != nil {
		preferred = strings.TrimSpace(policy.Prefer)
	}
	if preferred == "" && len(protocols) > 0 {
		preferred = strings.TrimSpace(protocols[0].Channel)
	}
	payload := buildTenantInvocationPayload(record.CapabilityID, preferred)
	debug := PlatformCapabilityDebugExamples{
		TenantInvocationCurl:    buildTenantInvocationCurl(payload),
		TenantInvocationPayload: payload,
	}
	return PlatformCapabilityDTO{
		CapabilityID:      record.CapabilityID,
		Title:             record.Title,
		Description:       record.Description,
		Module:            moduleMetadata.Module,
		Source:            capabilitycatalog.CapabilitySource(record),
		PluginID:          record.PluginID,
		PluginVersion:     record.PluginVersion,
		Docs:              moduleMetadata.Docs,
		CapabilitiesHash:  record.CapabilitiesHash,
		PreferredProtocol: preferred,
		Protocols:         protocols,
		DebugExamples:     debug,
	}
}

// NewPlatformCapabilityModuleDTO seeds a module DTO with defaults.
func NewPlatformCapabilityModuleDTO(key string) PlatformCapabilityModuleDTO {
	display, desc := resolveModulePresentation(key)
	return PlatformCapabilityModuleDTO{
		Module:       key,
		DisplayName:  display,
		Description:  desc,
		Capabilities: []PlatformCapabilityDTO{},
	}
}

// NormalizePlatformModuleKey normalizes incoming module identifiers for lookups.
func NormalizePlatformModuleKey(value string) string {
	return normalizeModuleKey(value)
}

type platformAnnotationPayload struct {
	Source      string   `json:"source"`
	Module      string   `json:"module"`
	Docs        []string `json:"docs"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
}

func parsePlatformAnnotations(record *modelregistry.CapabilityRecord) platformAnnotationPayload {
	payload := platformAnnotationPayload{}
	if record == nil {
		return payload
	}
	if len(record.Annotations) > 0 {
		_ = json.Unmarshal(record.Annotations, &payload)
	}
	payload.Module = normalizeModuleKey(payload.Module)
	if payload.Module == "" {
		payload.Module = deriveModuleFromCapability(record.CapabilityID)
	}
	return payload
}

func deriveModuleFromCapability(capabilityID string) string {
	parts := strings.Split(capabilityID, ".")
	for _, part := range parts {
		part = strings.TrimSpace(strings.ToLower(part))
		if part == "" {
			continue
		}
		if part == "com" || part == "corex" {
			continue
		}
		return normalizeModuleKey(part)
	}
	return "corex"
}

func normalizeModuleKey(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	return value
}

func resolveModulePresentation(key string) (string, string) {
	switch key {
	case "media":
		return "Media Assets Management", "PowerX 底座媒资/存储能力"
	case "event_fabric":
		return "Event Fabric", "事件发布与订阅能力"
	case "scheduler":
		return "Scheduler", "Workflow/Scheduler 调度器"
	case "knowledge":
		return "Knowledge Space", "知识库能力"
	case "workflow":
		return "Workflow", "Workflow Builder & Engine 能力"
	default:
		if key == "" {
			return "CoreX", ""
		}
		return capitalizeFirst(key), ""
	}
}

func capitalizeFirst(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes)
}

func buildTenantInvocationPayload(capabilityID, preferred string) map[string]interface{} {
	idempotency := fmt.Sprintf("demo-%s", sanitizeCapabilityID(capabilityID))
	body := map[string]interface{}{
		"capability_id":   capabilityID,
		"idempotency_key": idempotency,
		"payload":         map[string]interface{}{},
	}
	if preferred != "" {
		body["preferred_protocol"] = preferred
	}
	return body
}

func sanitizeCapabilityID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, ".", "-")
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")
	return value
}

func buildTenantInvocationCurl(body map[string]interface{}) string {
	if len(body) == 0 {
		return ""
	}
	raw, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return ""
	}
	return fmt.Sprintf("curl -X POST \"$POWERX_BASE_URL/tenant/invocations\" \\\n  -H \"Authorization: Bearer $TENANT_TOKEN\" \\\n  -H \"X-PowerX-Tenant: $TENANT_UUID\" \\\n  -H \"Content-Type: application/json\" \\\n  -d '%s'", string(raw))
}
