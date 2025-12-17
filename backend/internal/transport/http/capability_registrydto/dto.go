package capability_registrydto

import (
	"encoding/json"
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
