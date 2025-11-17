package knowledge_space

import (
	"errors"
	"net/http"
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
	SpaceID  string `json:"spaceId" binding:"required,uuid4"`
	Detected int    `json:"detected" binding:"required,min=1"`
}

type decayRestoreRequest struct {
	TaskID        string `json:"taskId" binding:"required,uuid4"`
	Notes         string `json:"notes"`
	FalsePositive bool   `json:"falsePositive"`
}

func (h *DecayHandler) Scan(c *gin.Context) {
	var req decayScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	spaceID, _ := uuid.Parse(req.SpaceID)
	tasks, err := h.svc.RunScan(c.Request.Context(), spaceID, req.Detected)
	if err != nil {
		h.handleError(c, err)
		return
	}
	now := time.Now()
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{
		"tasks":   toDecayTaskDTO(tasks, now),
		"metrics": gin.H{"knowledge.decay.detected": len(tasks)},
	})
}

func (h *DecayHandler) Restore(c *gin.Context) {
	var req decayRestoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	taskID, _ := uuid.Parse(req.TaskID)
	task, err := h.svc.Restore(c.Request.Context(), taskID, req.Notes, req.FalsePositive)
	if err != nil {
		h.handleError(c, err)
		return
	}
	dto.ResponseSuccess(c, task)
}

func (h *DecayHandler) Status(c *gin.Context) {
	spaceParam := strings.TrimSpace(c.Query("spaceId"))
	spaceID, err := uuid.Parse(spaceParam)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid spaceId", err)
		return
	}
	tasks, err := h.svc.ListOpen(c.Request.Context(), spaceID)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, err.Error(), err)
		return
	}
	severityFilter := strings.ToLower(strings.TrimSpace(c.Query("severity")))
	filtered := filterTasksBySeverity(tasks, severityFilter)
	now := time.Now()
	summary := gin.H{
		"knowledge.gap.backlog": len(filtered),
		"severity":              severityBuckets(filtered),
	}
	if severityFilter != "" {
		summary["filter"] = severityFilter
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
