package knowledge_space

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	decay "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/decay_guard"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// DecayHandler exposes decay endpoints.
type DecayHandler struct {
	svc *decay.Service
}

func NewDecayHandler(deps *shared.Deps) *DecayHandler {
	if deps == nil || deps.KnowledgeSpace == nil || deps.KnowledgeSpace.DecayGuard == nil {
		return nil
	}
	return &DecayHandler{svc: deps.KnowledgeSpace.DecayGuard}
}

type decayScanRequest struct {
	SpaceID     string `json:"spaceId" binding:"required,uuid4"`
	Detected    int    `json:"detected" binding:"required,min=1"`
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Reason      string `json:"reason"`
	AssignedTo  string `json:"assignedTo"`
	RequestedBy string `json:"requestedBy"`
}

type decayRestoreRequest struct {
	TaskID        string `json:"taskId" binding:"required,uuid4"`
	Notes         string `json:"notes"`
	FalsePositive bool   `json:"falsePositive"`
	ApprovedBy    string `json:"approvedBy"`
	Reason        string `json:"reason"`
}

func (h *DecayHandler) Scan(c *gin.Context) {
	if !flagEnabled("PX_KNOWLEDGE_DECAY_GUARD") {
		dto.ResponseError(c, http.StatusForbidden, "decay guard disabled", nil)
		return
	}
	var req decayScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	if _, ok := tenantUUIDFromContext(c); !ok {
		return
	}
	spaceID, _ := uuid.Parse(req.SpaceID)
	tasks, err := h.svc.RunScanWithInput(c.Request.Context(), decay.RunScanInput{
		SpaceID:     spaceID,
		Detected:    req.Detected,
		Category:    req.Category,
		Severity:    req.Severity,
		Reason:      req.Reason,
		AssignedTo:  req.AssignedTo,
		RequestedBy: req.RequestedBy,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}
	now := time.Now()
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{
		"tasks":   toDecayTaskDTO(tasks, now),
		"metrics": gin.H{
			"knowledge.decay.detected": len(tasks),
			"knowledge.gap.backlog":   len(tasks),
		},
	})
}

func (h *DecayHandler) Restore(c *gin.Context) {
	if !flagEnabled("PX_KNOWLEDGE_RESTORE_FLOW") {
		dto.ResponseError(c, http.StatusForbidden, "restore flow disabled", nil)
		return
	}
	var req decayRestoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	if _, ok := tenantUUIDFromContext(c); !ok {
		return
	}
	if req.FalsePositive && strings.TrimSpace(req.Reason) == "" && strings.TrimSpace(req.Notes) == "" {
		dto.ResponseError(c, http.StatusBadRequest, "误判恢复需要提供 reason 或 notes", decay.ErrInvalidInput)
		return
	}
	taskID, _ := uuid.Parse(req.TaskID)
	task, err := h.svc.RestoreWithInput(c.Request.Context(), decay.RestoreInput{
		TaskID:        taskID,
		Notes:         req.Notes,
		FalsePositive: req.FalsePositive,
		ApprovedBy:    req.ApprovedBy,
		Reason:        req.Reason,
	})
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, task)
}

func (h *DecayHandler) Status(c *gin.Context) {
	severityFilter := strings.ToLower(strings.TrimSpace(c.Query("severity")))
	exportFormat := strings.ToLower(strings.TrimSpace(c.Query("export")))
	tenantUUID, ok := tenantUUIDFromContext(c)
	if !ok {
		return
	}

	spaceParam := strings.TrimSpace(c.Query("spaceId"))
	var (
		filtered []*model.DecayTask
		err      error
	)
	if spaceParam == "" {
		filtered, err = h.svc.ListOpenByTenant(c.Request.Context(), tenantUUID, severityFilter)
	} else {
		spaceID, perr := uuid.Parse(spaceParam)
		if perr != nil {
			dto.ResponseError(c, http.StatusBadRequest, "invalid spaceId", perr)
			return
		}
		tasks, serr := h.svc.ListOpen(c.Request.Context(), spaceID)
		if serr != nil {
			dto.ResponseError(c, http.StatusInternalServerError, serr.Error(), serr)
			return
		}
		filtered = filterTasksBySeverity(tasks, severityFilter)
		err = nil
	}
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, err.Error(), err)
		return
	}
	now := time.Now()
	summary := gin.H{
		"knowledge.gap.backlog": len(filtered),
		"severity":              severityBuckets(filtered),
	}
	if severityFilter != "" {
		summary["filter"] = severityFilter
	}
	if exportFormat == "csv" {
		writeDecayCSV(c, filtered, now)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"tasks":   toDecayTaskDTO(filtered, now),
		"metrics": summary,
	})
}

func (h *DecayHandler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, decay.ErrInvalidInput):
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
	case errors.Is(err, decay.ErrTaskNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		dto.ResponseError(c, http.StatusNotFound, err.Error(), err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, err.Error(), err)
	}
}

type decayTaskDTO struct {
	UUID                string    `json:"uuid"`
	SpaceUUID           string    `json:"space_uuid"`
	Category            string    `json:"category"`
	Severity            string    `json:"severity"`
	Status              string    `json:"status"`
	DetectedAt          time.Time `json:"detected_at"`
	SLADueAt            time.Time `json:"sla_due_at"`
	FalsePositive       bool      `json:"false_positive"`
	SLARemainingMinutes int64     `json:"sla_remaining_minutes"`
}

func toDecayTaskDTO(tasks []*model.DecayTask, now time.Time) []decayTaskDTO {
	if len(tasks) == 0 {
		return []decayTaskDTO{}
	}
	result := make([]decayTaskDTO, 0, len(tasks))
	for _, task := range tasks {
		if task == nil {
			continue
		}
		result = append(result, decayTaskDTO{
			UUID:                task.UUID.String(),
			SpaceUUID:           task.SpaceUUID.String(),
			Category:            task.Category,
			Severity:            task.Severity,
			Status:              task.Status,
			DetectedAt:          task.DetectedAt,
			SLADueAt:            task.SLADueAt,
			FalsePositive:       task.FalsePositive,
			SLARemainingMinutes: slaRemainingMinutes(now, task.SLADueAt),
		})
	}
	return result
}

func filterTasksBySeverity(tasks []*model.DecayTask, severity string) []*model.DecayTask {
	if severity == "" {
		return tasks
	}
	filtered := make([]*model.DecayTask, 0, len(tasks))
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if strings.EqualFold(task.Severity, severity) {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

func severityBuckets(tasks []*model.DecayTask) map[string]int {
	if len(tasks) == 0 {
		return map[string]int{}
	}
	buckets := make(map[string]int)
	for _, task := range tasks {
		if task == nil {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(task.Severity))
		if key == "" {
			key = "unspecified"
		}
		buckets[key]++
	}
	return buckets
}

func slaRemainingMinutes(now time.Time, due time.Time) int64 {
	if due.IsZero() {
		return 0
	}
	diff := due.Sub(now)
	if diff <= 0 {
		return 0
	}
	mins := diff / time.Minute
	if diff%time.Minute != 0 {
		mins++
	}
	return int64(mins)
}

func flagEnabled(flag string) bool {
	flag = strings.TrimSpace(flag)
	if flag == "" {
		return true
	}
	value := strings.TrimSpace(os.Getenv(flag))
	if value == "" {
		return true
	}
	value = strings.ToLower(value)
	return value == "1" || value == "true" || value == "enabled" || value == "on" || value == "yes"
}

func writeDecayCSV(c *gin.Context, tasks []*model.DecayTask, now time.Time) {
	var b strings.Builder
	b.WriteString("task_id,space_id,category,severity,status,detected_at,sla_due_at,sla_remaining_minutes,false_positive\n")
	for _, task := range tasks {
		if task == nil {
			continue
		}
		b.WriteString(task.UUID.String())
		b.WriteByte(',')
		b.WriteString(task.SpaceUUID.String())
		b.WriteByte(',')
		b.WriteString(sanitizeCSV(task.Category))
		b.WriteByte(',')
		b.WriteString(sanitizeCSV(task.Severity))
		b.WriteByte(',')
		b.WriteString(sanitizeCSV(task.Status))
		b.WriteByte(',')
		b.WriteString(task.DetectedAt.UTC().Format(time.RFC3339Nano))
		b.WriteByte(',')
		b.WriteString(task.SLADueAt.UTC().Format(time.RFC3339Nano))
		b.WriteByte(',')
		b.WriteString(int64ToString(slaRemainingMinutes(now, task.SLADueAt)))
		b.WriteByte(',')
		if task.FalsePositive {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		b.WriteByte('\n')
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.String(http.StatusOK, b.String())
}

func sanitizeCSV(input string) string {
	input = strings.ReplaceAll(input, "\"", "\"\"")
	if strings.ContainsAny(input, ",\n\r") {
		return "\"" + input + "\""
	}
	return input
}

func int64ToString(v int64) string {
	return strconv.FormatInt(v, 10)
}
