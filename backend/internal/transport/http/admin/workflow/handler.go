package workflow

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"

	"github.com/ArtisanCloud/PowerX/internal/service/workflow"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	modelworkflow "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/workflow"
	workflowrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/workflow"
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
	Version     int32  `json:"version"`
	PublishedBy string `json:"published_by" validate:"omitempty,uuid4"`
	ChangeNote  string `json:"change_note"`
}

// StartInstanceRequest DTO.
type StartInstanceRequest struct {
	DefinitionUUID    string            `json:"definition_uuid" validate:"required,uuid4"`
	DefinitionVersion int32             `json:"definition_version"`
	Initiator         string            `json:"initiator" validate:"omitempty,uuid4"`
	Input             map[string]any    `json:"input"`
	Tags              map[string]string `json:"tags"`
	CorrelationID     string            `json:"correlation_id"`
}

type ControlInstanceRequest struct {
	Action       string         `json:"action" validate:"required"`
	StepID       string         `json:"step_id"`
	AssignmentID uint64         `json:"assignment_id"`
	Reason       string         `json:"reason"`
	Payload      map[string]any `json:"payload"`
}

type ValidateDefinitionRequest struct {
	Steps []workflow.StepDefinition `json:"steps" validate:"required,dive"`
}

type HumanReviewActionRequest struct {
	Action  string         `json:"action" validate:"required"`
	Comment string         `json:"comment"`
	Payload map[string]any `json:"payload"`
}

type WorkflowPackSeedRequest struct {
	Keys []string `json:"keys"`
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

	tenantUUID, ok := requireTenantContext(c)
	if !ok {
		return
	}

	definition, err := h.svc.CreateDefinition(c.Request.Context(), workflow.CreateDefinitionInput{
		TenantUUID:         tenantUUID,
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

	dto.ResponseSuccessWithStatus(c, http.StatusCreated, mapDefinition(definition, tenantUUID))
}

func (h *Handler) PublishDefinition(c *gin.Context) {
	if h.svc == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("workflow service unavailable", nil))
		return
	}

	definitionUUID, err := uuid.Parse(c.Param("definition_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid definition_uuid", err))
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

	tenantUUID, ok := requireTenantContext(c)
	if !ok {
		return
	}

	result, err := h.svc.PublishDefinition(c.Request.Context(), workflow.PublishDefinitionInput{
		TenantUUID:     tenantUUID,
		DefinitionUUID: definitionUUID,
		Version:        req.Version,
		PublishedBy:    publisher,
		ChangeNote:     req.ChangeNote,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("publish definition failed", err))
		return
	}

	dto.ResponseSuccess(c, mapDefinition(result, tenantUUID))
}

func (h *Handler) ListDefinitions(c *gin.Context) {
	if h.svc == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("workflow service unavailable", nil))
		return
	}

	tenantUUID, ok := requireTenantContext(c)
	if !ok {
		return
	}

	status := c.QueryArray("status")
	keyword := c.Query("keyword")
	limit, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	definitions, total, err := h.svc.ListDefinitions(c.Request.Context(), tenantUUID, status, keyword, limit, offset)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("list definitions failed", err))
		return
	}

	items := make([]map[string]any, 0, len(definitions))
	for i := range definitions {
		items = append(items, mapDefinition(&definitions[i], tenantUUID))
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

	tenantUUID, ok := requireTenantContext(c)
	if !ok {
		return
	}

	definitionUUID, err := uuid.Parse(c.Param("definition_uuid"))
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

	definition, err := h.svc.GetDefinition(c.Request.Context(), tenantUUID, definitionUUID, versionPtr)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("definition not found", err))
		return
	}

	dto.ResponseSuccess(c, mapDefinition(definition, tenantUUID))
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

	defID, err := uuid.Parse(req.DefinitionUUID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid definition_uuid", err))
		return
	}

	tenantUUID, ok := requireTenantContext(c)
	if !ok {
		return
	}

	instance, err := h.svc.StartInstance(c.Request.Context(), workflow.StartInstanceInput{
		TenantUUID:        tenantUUID,
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

	if _, err := h.svc.DrainDueSteps(c.Request.Context(), workflow.DrainDueStepsOptions{
		InstanceUUID:  instance.UUID,
		LeaseOwner:    "workflow-http-start",
		MaxIterations: 50,
	}); err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("run workflow instance failed", err))
		return
	}

	updated, records, err := h.svc.GetInstance(c.Request.Context(), tenantUUID, instance.UUID, true)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("instance not found", err))
		return
	}

	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, mapInstance(updated, records, tenantUUID))
}

func (h *Handler) GetInstance(c *gin.Context) {
	if h.svc == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("workflow service unavailable", nil))
		return
	}

	tenantUUID, ok := requireTenantContext(c)
	if !ok {
		return
	}

	instanceUUID, err := uuid.Parse(c.Param("instance_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid instance id", err))
		return
	}

	includeSteps := c.DefaultQuery("include_steps", "false") == "true"

	instance, records, err := h.svc.GetInstance(c.Request.Context(), tenantUUID, instanceUUID, includeSteps)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("instance not found", err))
		return
	}

	dto.ResponseSuccess(c, mapInstance(instance, records, tenantUUID))
}

func (h *Handler) ListInstances(c *gin.Context) {
	if h.svc == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("workflow service unavailable", nil))
		return
	}
	tenantUUID, ok := requireTenantContext(c)
	if !ok {
		return
	}

	var definitionUUID uuid.UUID
	if raw := strings.TrimSpace(c.Query("definition_uuid")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			dto.RespondErrorFrom(c, dto.NewBadRequest("invalid definition_uuid", err))
			return
		}
		definitionUUID = parsed
	}

	page := parsePositiveInt(c.Query("page"), 1)
	pageSize := parsePositiveInt(c.Query("page_size"), 20)
	instances, total, err := h.svc.ListInstances(c.Request.Context(), workflowrepo.InstanceListFilter{
		TenantUUID:     tenantUUID,
		DefinitionUUID: definitionUUID,
		State:          strings.TrimSpace(c.Query("state")),
		Page:           page,
		PageSize:       pageSize,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("list instances failed", err))
		return
	}

	items := make([]map[string]any, 0, len(instances))
	includeSteps := c.DefaultQuery("include_steps", "false") == "true"
	for i := range instances {
		var records []modelworkflow.WorkflowStepRecord
		if includeSteps {
			_, stepRecords, stepErr := h.svc.GetInstance(c.Request.Context(), tenantUUID, instances[i].UUID, true)
			if stepErr != nil {
				dto.RespondErrorFrom(c, dto.NewBadRequest("load instance steps failed", stepErr))
				return
			}
			records = stepRecords
		}
		items = append(items, mapInstance(&instances[i], records, tenantUUID))
	}

	dto.ResponseSuccess(c, map[string]any{
		"items": items,
		"total": total,
	})
}

func (h *Handler) ExportInstances(c *gin.Context) {
	if h.svc == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("workflow service unavailable", nil))
		return
	}

	tenantUUID, ok := requireTenantContext(c)
	if !ok {
		return
	}

	var definitionUUID *uuid.UUID
	if definitionParam := strings.TrimSpace(c.Query("definition_uuid")); definitionParam != "" {
		id, parseErr := uuid.Parse(definitionParam)
		if parseErr != nil {
			dto.RespondErrorFrom(c, dto.NewBadRequest("invalid definition_uuid", parseErr))
			return
		}
		definitionUUID = &id
	}

	includeSteps := true
	if flag := strings.TrimSpace(c.Query("include_step_details")); flag != "" {
		includeSteps = !(flag == "false" || flag == "0")
	}

	var createdFromPtr, createdToPtr *time.Time
	if from := strings.TrimSpace(c.Query("created_from")); from != "" {
		timestamp, parseErr := time.Parse(time.RFC3339, from)
		if parseErr != nil {
			dto.RespondErrorFrom(c, dto.NewBadRequest("invalid created_from", parseErr))
			return
		}
		createdFromPtr = &timestamp
	}
	if to := strings.TrimSpace(c.Query("created_to")); to != "" {
		timestamp, parseErr := time.Parse(time.RFC3339, to)
		if parseErr != nil {
			dto.RespondErrorFrom(c, dto.NewBadRequest("invalid created_to", parseErr))
			return
		}
		createdToPtr = &timestamp
	}

	formatQuery := strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", "csv")))
	filter := workflow.ExportFilter{
		TenantUUID:         tenantUUID,
		DefinitionUUID:     definitionUUID,
		State:              strings.TrimSpace(c.Query("state")),
		CreatedFrom:        createdFromPtr,
		CreatedTo:          createdToPtr,
		IncludeStepDetails: includeSteps,
		Format:             workflow.ExportFormat(formatQuery),
	}

	result, err := h.svc.ExportInstances(c.Request.Context(), filter)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("export instances failed", err))
		return
	}

	rows := make([]map[string]any, 0, len(result.Rows))
	for _, row := range result.Rows {
		payload := map[string]any{
			"instance_uuid":      row.InstanceID,
			"definition_uuid":    row.DefinitionID,
			"definition_version": row.DefinitionVersion,
			"state":              row.State,
			"tenant_uuid":        row.TenantUUID,
			"correlation_id":     row.CorrelationID,
		}
		if row.StartedAt != nil {
			payload["started_at"] = row.StartedAt.Format(time.RFC3339)
		}
		if row.CompletedAt != nil {
			payload["completed_at"] = row.CompletedAt.Format(time.RFC3339)
		}
		if len(row.Steps) > 0 {
			stepPayload := make([]map[string]any, 0, len(row.Steps))
			for _, step := range row.Steps {
				item := map[string]any{
					"step_id":            step.StepID,
					"type":               step.Type,
					"state":              step.State,
					"subject_type":       step.SubjectType,
					"subject_id":         step.SubjectID,
					"attempts":           step.Attempts,
					"tool_grant_version": step.ToolGrantVersion,
					"last_error":         step.LastError,
				}
				if !step.LastTransitionAt.IsZero() {
					item["last_transition_at"] = step.LastTransitionAt.Format(time.RFC3339)
				}
				stepPayload = append(stepPayload, item)
			}
			payload["steps"] = stepPayload
		}
		rows = append(rows, payload)
	}

	dto.ResponseSuccess(c, map[string]any{
		"meta": map[string]any{
			"format":       string(result.Format),
			"generated_at": result.GeneratedAt.Format(time.RFC3339),
			"row_count":    len(rows),
		},
		"rows":         rows,
		"download_url": result.DownloadURL,
	})
}

func (h *Handler) ControlInstance(c *gin.Context) {
	if h.svc == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("workflow service unavailable", nil))
		return
	}

	tenantUUID, ok := requireTenantContext(c)
	if !ok {
		return
	}

	instanceUUID, err := uuid.Parse(c.Param("instance_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid instance id", err))
		return
	}

	var req ControlInstanceRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid payload", err))
		return
	}

	updated, err := h.svc.ControlInstance(c.Request.Context(), workflow.ControlInstanceInput{
		TenantUUID:   tenantUUID,
		InstanceUUID: instanceUUID,
		Action:       strings.ToLower(req.Action),
		StepID:       req.StepID,
		AssignmentID: req.AssignmentID,
		Reason:       req.Reason,
		Payload:      req.Payload,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("control instance failed", err))
		return
	}

	inst, records, err := h.svc.GetInstance(c.Request.Context(), tenantUUID, instanceUUID, true)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("load instance failed", err))
		return
	}

	if updated != nil {
		dto.ResponseSuccess(c, mapInstance(updated, records, tenantUUID))
		return
	}

	dto.ResponseSuccess(c, mapInstance(inst, records, tenantUUID))
}

func (h *Handler) ValidateDefinition(c *gin.Context) {
	var req ValidateDefinitionRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid payload", err))
		return
	}
	result, err := workflow.ValidateStepDefinitions(req.Steps)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("workflow.definition_invalid", err))
		return
	}
	dto.ResponseSuccess(c, map[string]any{
		"valid":          true,
		"start_step_ids": result.StartStepIDs,
	})
}

func (h *Handler) ListNodeCatalog(c *gin.Context) {
	items, err := h.svc.ListNodeCatalog(c.Request.Context())
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("list node catalog failed", err))
		return
	}
	out := make([]map[string]any, 0, len(items))
	for i := range items {
		out = append(out, mapNodeCatalogItem(items[i]))
	}
	dto.ResponseSuccess(c, map[string]any{"items": out})
}

func (h *Handler) GetNodeCatalogItem(c *gin.Context) {
	item, err := h.svc.GetNodeCatalogItem(c.Request.Context(), c.Param("node_kind"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("node catalog item not found", err))
		return
	}
	dto.ResponseSuccess(c, mapNodeCatalogItem(item))
}

func (h *Handler) ListHumanReviewTasks(c *gin.Context) {
	tenantUUID, ok := requireTenantContext(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	var instanceUUID uuid.UUID
	if raw := strings.TrimSpace(c.Query("workflow_instance_uuid")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			dto.RespondErrorFrom(c, dto.NewBadRequest("invalid workflow_instance_uuid", err))
			return
		}
		instanceUUID = parsed
	}
	tasks, total, err := h.svc.ListHumanReviewTasks(c.Request.Context(), workflow.HumanReviewListInput{
		TenantUUID:           tenantUUID,
		Status:               c.Query("status"),
		WorkflowInstanceUUID: instanceUUID,
		ReviewType:           c.Query("review_type"),
		Page:                 page,
		PageSize:             pageSize,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("list review tasks failed", err))
		return
	}
	items := make([]map[string]any, 0, len(tasks))
	for i := range tasks {
		items = append(items, mapHumanReviewTask(&tasks[i]))
	}
	dto.ResponseSuccess(c, map[string]any{"items": items, "total": total})
}

func (h *Handler) GetHumanReviewTask(c *gin.Context) {
	tenantUUID, ok := requireTenantContext(c)
	if !ok {
		return
	}
	taskUUID, err := uuid.Parse(c.Param("review_task_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid review_task_uuid", err))
		return
	}
	task, err := h.svc.GetHumanReviewTask(c.Request.Context(), tenantUUID, taskUUID)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("review task not found", err))
		return
	}
	dto.ResponseSuccess(c, mapHumanReviewTask(task))
}

func (h *Handler) ActHumanReviewTask(c *gin.Context) {
	tenantUUID, ok := requireTenantContext(c)
	if !ok {
		return
	}
	taskUUID, err := uuid.Parse(c.Param("review_task_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid review_task_uuid", err))
		return
	}
	var req HumanReviewActionRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid payload", err))
		return
	}
	reviewerUUID, err := uuid.Parse(strings.TrimSpace(reqctx.GetUserUUID(c.Request.Context())))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("reviewer user uuid missing from request context", err))
		return
	}
	task, err := h.svc.ActHumanReviewTask(c.Request.Context(), workflow.HumanReviewActionInput{
		TenantUUID:     tenantUUID,
		ReviewTaskUUID: taskUUID,
		Action:         req.Action,
		ReviewerUUID:   reviewerUUID,
		Comment:        req.Comment,
		Payload:        req.Payload,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("act review task failed", err))
		return
	}
	dto.ResponseSuccess(c, mapHumanReviewTask(task))
}

func (h *Handler) ListWorkflowPacks(c *gin.Context) {
	tenantUUID, ok := requireTenantContext(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	records, total, err := h.svc.ListWorkflowPacks(c.Request.Context(), tenantUUID, c.Query("keyword"), limit, offset)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("list workflow packs failed", err))
		return
	}
	items := make([]map[string]any, 0, len(records))
	for i := range records {
		items = append(items, mapWorkflowPackSeedRecord(&records[i]))
	}
	dto.ResponseSuccess(c, map[string]any{"items": items, "total": total})
}

func (h *Handler) SeedWorkflowPacks(c *gin.Context) {
	tenantUUID, ok := requireTenantContext(c)
	if !ok {
		return
	}
	var req WorkflowPackSeedRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid payload", err))
		return
	}
	result, err := h.svc.SeedWorkflowPacks(c.Request.Context(), workflow.WorkflowPackSeedInput{
		TenantUUID: tenantUUID,
		ConfigDir:  "config/workflow_packs",
		Keys:       req.Keys,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("seed workflow packs failed", err))
		return
	}
	seeded := make([]map[string]any, 0, len(result.Seeded))
	for i := range result.Seeded {
		seeded = append(seeded, mapWorkflowPackSeedRecord(&result.Seeded[i]))
	}
	dto.ResponseSuccess(c, map[string]any{"seeded": seeded, "skipped": result.Skipped})
}

func (h *Handler) GetWorkflowPack(c *gin.Context) {
	tenantUUID, ok := requireTenantContext(c)
	if !ok {
		return
	}
	record, err := h.svc.GetWorkflowPack(c.Request.Context(), tenantUUID, c.Param("workflow_key"))
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("workflow pack not found", err))
		return
	}
	dto.ResponseSuccess(c, mapWorkflowPackSeedRecord(record))
}

func mapDefinition(def *modelworkflow.WorkflowDefinition, tenantUUID string) map[string]any {
	if def == nil {
		return nil
	}
	return map[string]any{
		"uuid":                   def.UUID.String(),
		"tenant_uuid":            tenantUUID,
		"name":                   def.Name,
		"description":            def.Description,
		"version":                def.Version,
		"status":                 def.Status,
		"step_graph":             jsonToInterface(def.StepGraph),
		"default_retry_policy":   jsonToInterface(def.DefaultRetryPolicy),
		"compensation_policy":    jsonToInterface(def.CompensationPolicy),
		"sla_policy":             jsonToInterface(def.SlaPolicy),
		"metadata":               jsonToInterface(def.Metadata),
		"input_schema":           jsonToInterface(def.InputSchema),
		"workflow_pack_key":      def.WorkflowPackKey,
		"source_type":            def.SourceType,
		"checksum":               def.Checksum,
		"created_at":             def.CreatedAt,
		"updated_at":             def.UpdatedAt,
		"published_at":           def.PublishedAt,
		"archived_at":            def.ArchivedAt,
		"initial_context_schema": jsonToInterface(def.InitialContextSchema),
	}
}

func mapNodeCatalogItem(item workflow.NodeCatalogItem) map[string]any {
	return map[string]any{
		"node_kind":              item.NodeKind,
		"display_name_i18n_key":  item.DisplayNameI18nKey,
		"description_i18n_key":   item.DescriptionI18nKey,
		"category":               item.Category,
		"step_type":              item.StepType,
		"input_schema":           item.InputSchema,
		"output_schema":          item.OutputSchema,
		"config_schema":          item.ConfigSchema,
		"required_permissions":   item.RequiredPermissions,
		"required_capabilities":  item.RequiredCapabilities,
		"idempotency_required":   item.IdempotencyRequired,
		"compensation_supported": item.CompensationSupported,
		"source_status":          item.SourceStatus,
		"metadata":               item.Metadata,
	}
}

func mapHumanReviewTask(task *modelworkflow.HumanReviewTask) map[string]any {
	if task == nil {
		return nil
	}
	return map[string]any{
		"review_task_uuid":       task.UUID.String(),
		"tenant_uuid":            task.TenantUUID,
		"workflow_instance_uuid": task.WorkflowInstanceUUID.String(),
		"step_id":                task.StepID,
		"review_type":            task.ReviewType,
		"payload":                jsonToInterface(task.Payload),
		"approver_policy":        jsonToInterface(task.ApproverPolicy),
		"status":                 task.Status,
		"reviewer_user_uuid":     task.ReviewerUserUUID.String(),
		"decision":               task.Decision,
		"decision_payload":       jsonToInterface(task.DecisionPayload),
		"comment":                task.Comment,
		"due_at":                 task.DueAt,
		"completed_at":           task.CompletedAt,
	}
}

func mapWorkflowPackSeedRecord(record *modelworkflow.WorkflowPackSeedRecord) map[string]any {
	if record == nil {
		return nil
	}
	return map[string]any{
		"workflow_key":       record.WorkflowKey,
		"version":            record.Version,
		"definition_uuid":    record.DefinitionUUID.String(),
		"definition_version": record.DefinitionVersion,
		"checksum":           record.Checksum,
		"source":             record.Source,
		"seeded_at":          record.SeededAt,
	}
}

func mapInstance(inst *modelworkflow.WorkflowInstance, steps []modelworkflow.WorkflowStepRecord, tenantUUID string) map[string]any {
	if inst == nil {
		return nil
	}

	payload := map[string]any{
		"uuid":                inst.UUID.String(),
		"tenant_uuid":         tenantUUID,
		"definition_uuid":     inst.DefinitionUUID.String(),
		"definition_version":  inst.DefinitionVersion,
		"state":               inst.State,
		"input_context":       jsonToInterface(inst.InputContext),
		"runtime_context":     jsonToInterface(inst.RuntimeContext),
		"output_context":      jsonToInterface(inst.OutputContext),
		"tags":                jsonToStringMap(inst.Tags),
		"last_error":          inst.LastError,
		"agent_uuid":          inst.AgentUUID,
		"initiator_user_uuid": inst.InitiatorUserUUID,
		"trace_id":            inst.TraceID,
		"started_at":          inst.StartedAt,
		"completed_at":        inst.CompletedAt,
		"current_step_id":     inst.CurrentStepID,
		"next_heartbeat_due":  inst.NextHeartbeatDue,
		"last_transition_at":  inst.LastTransitionAt,
	}

	if len(steps) > 0 {
		stepItems := make([]map[string]any, 0, len(steps))
		for i := range steps {
			stepItems = append(stepItems, map[string]any{
				"id":              steps[i].ID,
				"step_id":         steps[i].StepID,
				"type":            steps[i].Type,
				"node_kind":       steps[i].NodeKind,
				"node_ref":        steps[i].NodeRef,
				"state":           steps[i].State,
				"subject_type":    steps[i].SubjectType,
				"subject_uuid":    steps[i].SubjectUUID,
				"tool_grant_id":   steps[i].ToolGrantID,
				"tool_grant_ver":  steps[i].ToolGrantVer,
				"attempt":         steps[i].Attempt,
				"input_mapping":   jsonToInterface(steps[i].InputMapping),
				"output_mapping":  jsonToInterface(steps[i].OutputMapping),
				"payload_in":      jsonToInterface(steps[i].PayloadIn),
				"payload_out":     jsonToInterface(steps[i].PayloadOut),
				"failure_reason":  steps[i].FailureReason,
				"error_code":      steps[i].ErrorCode,
				"error_message":   steps[i].ErrorMessage,
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

func parsePositiveInt(raw string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
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
