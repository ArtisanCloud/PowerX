package plugin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	pluginservice "github.com/ArtisanCloud/PowerX/internal/service/plugin"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/gin-gonic/gin"
)

type createDrainJobReq struct {
	Version string `json:"version"`
	Reason  string `json:"reason"`
	Mode    string `json:"mode"`
}

type cancelDrainBlockersReq struct {
	Reason            string   `json:"reason"`
	EventTaskIDs      []uint64 `json:"event_task_ids"`
	SchedulerJobUUIDs []string `json:"scheduler_job_uuids"`
}

type pluginDrainJobView struct {
	ID                  uint64                     `json:"id"`
	UUID                string                     `json:"uuid,omitempty"`
	JobID               string                     `json:"job_id"`
	PluginID            string                     `json:"plugin_id"`
	Version             string                     `json:"version,omitempty"`
	Scope               string                     `json:"scope"`
	Status              string                     `json:"status"`
	Reason              string                     `json:"reason,omitempty"`
	RequestedByRootUser uint64                     `json:"requested_by_root_user_id,omitempty"`
	AffectedTenantCount int64                      `json:"affected_tenant_count"`
	DrainedTenantCount  int64                      `json:"drained_tenant_count"`
	LastBlockerSummary  *pluginDrainBlockerSummary `json:"last_blocker_summary,omitempty"`
	CompletedAt         *time.Time                 `json:"completed_at,omitempty"`
	CreatedAt           time.Time                  `json:"createdAt"`
	UpdatedAt           time.Time                  `json:"updatedAt"`
}

type pluginDrainBlockerSummary struct {
	EventTaskCount    int64  `json:"event_task_count"`
	SchedulerJobCount int64  `json:"scheduler_job_count"`
	HasBlockers       bool   `json:"has_blockers"`
	PluginID          string `json:"plugin_id,omitempty"`
}

func toPluginDrainJobView(job *dbsetting.PluginDrainJob) *pluginDrainJobView {
	if job == nil {
		return nil
	}
	uuidText := ""
	if job.UUID.String() != "00000000-0000-0000-0000-000000000000" {
		uuidText = job.UUID.String()
	}
	return &pluginDrainJobView{
		ID:                  job.ID,
		UUID:                uuidText,
		JobID:               job.JobID,
		PluginID:            job.PluginID,
		Version:             job.Version,
		Scope:               job.Scope,
		Status:              job.Status,
		Reason:              job.Reason,
		RequestedByRootUser: job.RequestedByRootUser,
		AffectedTenantCount: job.AffectedTenantCount,
		DrainedTenantCount:  job.DrainedTenantCount,
		LastBlockerSummary:  toPluginDrainBlockerSummary(job),
		CompletedAt:         job.CompletedAt,
		CreatedAt:           job.CreatedAt,
		UpdatedAt:           job.UpdatedAt,
	}
}

func toPluginDrainJobViews(jobs []*dbsetting.PluginDrainJob) []*pluginDrainJobView {
	out := make([]*pluginDrainJobView, 0, len(jobs))
	for _, job := range jobs {
		if view := toPluginDrainJobView(job); view != nil {
			out = append(out, view)
		}
	}
	return out
}

func toPluginDrainBlockerSummary(job *dbsetting.PluginDrainJob) *pluginDrainBlockerSummary {
	if job == nil || len(job.LastBlockerJSON) == 0 {
		return nil
	}
	var payload struct {
		PluginID          string `json:"plugin_id"`
		EventTaskCount    int64  `json:"event_task_count"`
		SchedulerJobCount int64  `json:"scheduler_job_count"`
	}
	if err := json.Unmarshal(job.LastBlockerJSON, &payload); err != nil {
		return nil
	}
	total := payload.EventTaskCount + payload.SchedulerJobCount
	if total <= 0 {
		return nil
	}
	return &pluginDrainBlockerSummary{
		PluginID:          payload.PluginID,
		EventTaskCount:    payload.EventTaskCount,
		SchedulerJobCount: payload.SchedulerJobCount,
		HasBlockers:       true,
	}
}

func PluginDrainCreateHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		if id == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少插件ID", nil)
			return
		}
		var req createDrainJobReq
		if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
			dtoRequest.ResponseValidationError(c, err)
			return
		}
		var taskDriver event_bus.TaskDriver
		if deps != nil && deps.EventFabric != nil {
			taskDriver = deps.EventFabric.TaskDriver
		}
		svc := pluginservice.NewPluginDrainJobServiceWithTaskDriver(deps.DB, taskDriver)
		job, err := svc.CreateDrainJob(c.Request.Context(), pluginservice.CreateDrainJobInput{
			PluginID: id,
			Version:  req.Version,
			Reason:   req.Reason,
			Mode:     req.Mode,
		})
		if err != nil {
			dtoRequest.RespondErrorFrom(c, err)
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{"job": toPluginDrainJobView(job)})
	}
}

func PluginDrainListHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		if id == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少插件ID", nil)
			return
		}
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		var taskDriver event_bus.TaskDriver
		if deps != nil && deps.EventFabric != nil {
			taskDriver = deps.EventFabric.TaskDriver
		}
		svc := pluginservice.NewPluginDrainJobServiceWithTaskDriver(deps.DB, taskDriver)
		items, err := svc.ListDrainJobs(c.Request.Context(), id, limit)
		if err != nil {
			dtoRequest.RespondErrorFrom(c, err)
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{"items": toPluginDrainJobViews(items)})
	}
}

func PluginDrainGetHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		jobID := strings.TrimSpace(c.Param("job_id"))
		if jobID == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少 drain job id", nil)
			return
		}
		svc := pluginservice.NewPluginDrainJobService(deps.DB)
		job, err := svc.GetDrainJob(c.Request.Context(), jobID)
		if err != nil {
			dtoRequest.RespondErrorFrom(c, err)
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{"job": toPluginDrainJobView(job)})
	}
}

func PluginDrainRefreshHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		jobID := strings.TrimSpace(c.Param("job_id"))
		if jobID == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少 drain job id", nil)
			return
		}
		var taskDriver event_bus.TaskDriver
		if deps != nil && deps.EventFabric != nil {
			taskDriver = deps.EventFabric.TaskDriver
		}
		svc := pluginservice.NewPluginDrainJobServiceWithTaskDriver(deps.DB, taskDriver)
		job, err := svc.RefreshDrainJobProgress(c.Request.Context(), jobID)
		if err != nil {
			dtoRequest.RespondErrorFrom(c, err)
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{"job": toPluginDrainJobView(job)})
	}
}

func PluginDrainBlockersListHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		if id == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少插件ID", nil)
			return
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
		kind := strings.TrimSpace(c.DefaultQuery("kind", "event_task"))
		svc := pluginservice.NewPluginDrainJobService(deps.DB)
		result, err := svc.ListRuntimeBlockers(c.Request.Context(), pluginservice.ListDrainBlockersInput{
			PluginID: id,
			Kind:     kind,
			Page:     page,
			PageSize: pageSize,
		})
		if err != nil {
			dtoRequest.RespondErrorFrom(c, err)
			return
		}
		dtoRequest.ResponseSuccess(c, result)
	}
}

func PluginDrainCancelBlockersHandler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.Param("id"))
		if id == "" {
			dtoRequest.ResponseError(c, http.StatusBadRequest, "缺少插件ID", nil)
			return
		}
		var req cancelDrainBlockersReq
		if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
			dtoRequest.ResponseValidationError(c, err)
			return
		}
		var taskDriver event_bus.TaskDriver
		if deps != nil && deps.EventFabric != nil {
			taskDriver = deps.EventFabric.TaskDriver
		}
		svc := pluginservice.NewPluginDrainJobServiceWithTaskDriver(deps.DB, taskDriver)
		result, err := svc.CancelRuntimeBlockers(c.Request.Context(), pluginservice.CancelDrainBlockersInput{
			PluginID:          id,
			Reason:            req.Reason,
			EventTaskIDs:      req.EventTaskIDs,
			SchedulerJobUUIDs: req.SchedulerJobUUIDs,
		})
		if err != nil {
			dtoRequest.RespondErrorFrom(c, err)
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{
			"plugin_id":                result.PluginID,
			"cancelled_scheduler_jobs": result.CancelledSchedulerJob,
			"cancelled_event_tasks":    result.CancelledEventTask,
			"drain_jobs":               toPluginDrainJobViews(result.DrainJobs),
		})
	}
}
