package workflow

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"

	"github.com/ArtisanCloud/PowerX/internal/service/workflow"
	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// Handler 提供工作流定义与实例的 HTTP API。
type Handler struct {
	svc *workflow.Service
}

// NewHandler 构建 Handler。
func NewHandler(svc *workflow.Service) *Handler {
	return &Handler{svc: svc}
}

// CreateDefinitionRequest DTO.
type CreateDefinitionRequest struct {
	TenantID           uint64                    `json:"tenant_id" validate:"required"`
	Name               string                    `json:"name" validate:"required"`
	Description        string                    `json:"description"`
	CreatedBy          string                    `json:"created_by" validate:"omitempty,uuid4"`
	Steps              []workflow.StepDefinition `json:"steps" validate:"required,dive"`
	DefaultRetryPolicy map[string]any            `json:"default_retry_policy"`
	CompensationPolicy map[string]any            `json:"compensation_policy"`
	SlaPolicy          map[string]any            `json:"sla_policy"`
	Metadata           map[string]any            `json:"metadata"`
}

// PublishDefinitionRequest DTO.
type PublishDefinitionRequest struct {
	TenantID    uint64 `json:"tenant_id" validate:"required"`
	Version     int32  `json:"version"`
	PublishedBy string `json:"published_by" validate:"omitempty,uuid4"`
	ChangeNote  string `json:"change_note"`
}

// StartInstanceRequest DTO.
type StartInstanceRequest struct {
	TenantID          uint64            `json:"tenant_id" validate:"required"`
	DefinitionID      string            `json:"definition_id" validate:"required,uuid4"`
	DefinitionVersion int32             `json:"definition_version"`
	Initiator         string            `json:"initiator" validate:"omitempty,uuid4"`
	Input             map[string]any    `json:"input"`
	Tags              map[string]string `json:"tags"`
	CorrelationID     string            `json:"correlation_id"`
}

func (h *Handler) CreateDefinition(c *gin.Context) {
	if h.svc == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("workflow service unavailable", nil))
		return
	}

	var req CreateDefinitionRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid payload", err))
		return
	}

	createdBy := uuid.New()
	if req.CreatedBy != "" {
		var err error
		createdBy, err = uuid.Parse(req.CreatedBy)
		if err != nil {
			dto.RespondErrorFrom(c, dto.NewBadRequest("invalid created_by", err))
			return
		}
	}

	definition, err := h.svc.CreateDefinition(c.Request.Context(), workflow.CreateDefinitionInput{
		TenantID:           req.TenantID,
		Name:               strings.TrimSpace(req.Name),
		Description:        strings.TrimSpace(req.Description),
		CreatedBy:          createdBy,
		Steps:              req.Steps,
		DefaultRetryPolicy: req.DefaultRetryPolicy,
		CompensationPolicy: req.CompensationPolicy,
		SlaPolicy:          req.SlaPolicy,
		Metadata:           req.Metadata,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("create workflow definition failed", err))
		return
	}

	dto.ResponseSuccessWithStatus(c, http.StatusCreated, mapDefinition(definition))
}

func (h *Handler) PublishDefinition(c *gin.Context) {
	if h.svc == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("workflow service unavailable", nil))
		return
	}

	definitionUUID, err := uuid.Parse(c.Param("definitionId"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid definition_id", err))
		return
	}

	var req PublishDefinitionRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid payload", err))
		return
	}
	publisher := uuid.New()
	if req.PublishedBy != "" {
		publisher, err = uuid.Parse(req.PublishedBy)
		if err != nil {
			dto.RespondErrorFrom(c, dto.NewBadRequest("invalid published_by", err))
			return
		}
	}

	result, err := h.svc.PublishDefinition(c.Request.Context(), workflow.PublishDefinitionInput{
		TenantID:       req.TenantID,
		DefinitionUUID: definitionUUID,
		Version:        req.Version,
		PublishedBy:    publisher,
		ChangeNote:     req.ChangeNote,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("publish definition failed", err))
		return
	}

	dto.ResponseSuccess(c, mapDefinition(result))
}

func (h *Handler) ListDefinitions(c *gin.Context) {
	if h.svc == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("workflow service unavailable", nil))
		return
	}

	tenantID, err := strconv.ParseUint(c.Query("tenant_id"), 10, 64)
	if err != nil || tenantID == 0 {
		dto.RespondErrorFrom(c, dto.NewBadRequest("tenant_id is required", err))
		return
	}

	status := c.QueryArray("status")
	keyword := c.Query("keyword")
	limit, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	definitions, total, err := h.svc.ListDefinitions(c.Request.Context(), uint64(tenantID), status, keyword, limit, offset)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("list definitions failed", err))
		return
	}

	items := make([]map[string]any, 0, len(definitions))
	for i := range definitions {
		items = append(items, mapDefinition(&definitions[i]))
	}

	dto.ResponseSuccess(c, map[string]any{
		"items": items,
		"total": total,
	})
}

func (h *Handler) GetDefinition(c *gin.Context) {
	if h.svc == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("workflow service unavailable", nil))
		return
	}

	tenantID, err := strconv.ParseUint(c.Query("tenant_id"), 10, 64)
	if err != nil || tenantID == 0 {
		dto.RespondErrorFrom(c, dto.NewBadRequest("tenant_id is required", err))
		return
	}

	definitionUUID, err := uuid.Parse(c.Param("definitionId"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid definition id", err))
		return
	}

	var versionPtr *int32
	if v := c.Query("version"); v != "" {
		val, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			dto.RespondErrorFrom(c, dto.NewBadRequest("invalid version", err))
			return
		}
		tmp := int32(val)
		versionPtr = &tmp
	}

	definition, err := h.svc.GetDefinition(c.Request.Context(), uint64(tenantID), definitionUUID, versionPtr)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("definition not found", err))
		return
	}

	dto.ResponseSuccess(c, mapDefinition(definition))
}

func (h *Handler) StartInstance(c *gin.Context) {
	if h.svc == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("workflow service unavailable", nil))
		return
	}

	var req StartInstanceRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid payload", err))
		return
	}

	defID, err := uuid.Parse(req.DefinitionID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid definition_id", err))
		return
	}

	instance, err := h.svc.StartInstance(c.Request.Context(), workflow.StartInstanceInput{
		TenantID:          req.TenantID,
		DefinitionUUID:    defID,
		DefinitionVersion: req.DefinitionVersion,
		Input:             req.Input,
		Tags:              req.Tags,
		CorrelationID:     strings.TrimSpace(req.CorrelationID),
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("start instance failed", err))
		return
	}

	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, mapInstance(instance, nil))
}

func (h *Handler) GetInstance(c *gin.Context) {
	if h.svc == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("workflow service unavailable", nil))
		return
	}

	tenantID, err := strconv.ParseUint(c.Query("tenant_id"), 10, 64)
	if err != nil || tenantID == 0 {
		dto.RespondErrorFrom(c, dto.NewBadRequest("tenant_id is required", err))
		return
	}

	instanceUUID, err := uuid.Parse(c.Param("instanceId"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid instance id", err))
		return
	}

	includeSteps := c.DefaultQuery("include_steps", "false") == "true"

	instance, records, err := h.svc.GetInstance(c.Request.Context(), uint64(tenantID), instanceUUID, includeSteps)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("instance not found", err))
		return
	}

	dto.ResponseSuccess(c, mapInstance(instance, records))
}

func mapDefinition(def *modelworkflow.WorkflowDefinition) map[string]any {
	if def == nil {
		return nil
	}
	return map[string]any{
		"uuid":                   def.UUID.String(),
		"tenant_id":              def.TenantID,
		"name":                   def.Name,
		"description":            def.Description,
		"version":                def.Version,
		"status":                 def.Status,
		"step_graph":             jsonToInterface(def.StepGraph),
		"default_retry_policy":   jsonToInterface(def.DefaultRetryPolicy),
		"compensation_policy":    jsonToInterface(def.CompensationPolicy),
		"sla_policy":             jsonToInterface(def.SlaPolicy),
		"metadata":               jsonToInterface(def.Metadata),
		"created_at":             def.CreatedAt,
		"updated_at":             def.UpdatedAt,
		"published_at":           def.PublishedAt,
		"archived_at":            def.ArchivedAt,
		"initial_context_schema": jsonToInterface(def.InitialContextSchema),
	}
}

func mapInstance(inst *modelworkflow.WorkflowInstance, steps []modelworkflow.WorkflowStepRecord) map[string]any {
	if inst == nil {
		return nil
	}

	payload := map[string]any{
		"uuid":               inst.UUID.String(),
		"tenant_id":          inst.TenantID,
		"definition_uuid":    inst.DefinitionUUID.String(),
		"definition_version": inst.DefinitionVersion,
		"state":              inst.State,
		"input_context":      jsonToInterface(inst.InputContext),
		"output_context":     jsonToInterface(inst.OutputContext),
		"tags":               jsonToStringMap(inst.Tags),
		"last_error":         inst.LastError,
		"started_at":         inst.StartedAt,
		"completed_at":       inst.CompletedAt,
		"current_step_id":    inst.CurrentStepID,
		"next_heartbeat_due": inst.NextHeartbeatDue,
		"last_transition_at": inst.LastTransitionAt,
	}

	if len(steps) > 0 {
		stepItems := make([]map[string]any, 0, len(steps))
		for i := range steps {
			stepItems = append(stepItems, map[string]any{
				"id":              steps[i].ID,
				"step_id":         steps[i].StepID,
				"type":            steps[i].Type,
				"state":           steps[i].State,
				"subject_type":    steps[i].SubjectType,
				"subject_uuid":    steps[i].SubjectUUID,
				"tool_grant_id":   steps[i].ToolGrantID,
				"tool_grant_ver":  steps[i].ToolGrantVer,
				"attempt":         steps[i].Attempt,
				"payload_in":      jsonToInterface(steps[i].PayloadIn),
				"payload_out":     jsonToInterface(steps[i].PayloadOut),
				"failure_reason":  steps[i].FailureReason,
				"scheduled_at":    steps[i].ScheduledAt,
				"started_at":      steps[i].StartedAt,
				"completed_at":    steps[i].CompletedAt,
				"last_transition": steps[i].LastTransition,
				"awaiting_human":  steps[i].AwaitingHuman,
			})
		}
		payload["steps"] = stepItems
	}
	return payload
}

func jsonToInterface(data datatypes.JSON) any {
	if len(data) == 0 {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return map[string]any{}
	}
	return v
}

func jsonToStringMap(data datatypes.JSON) map[string]string {
	if len(data) == 0 {
		return map[string]string{}
	}
	var v map[string]string
	if err := json.Unmarshal(data, &v); err != nil {
		return map[string]string{}
	}
	return v
}
