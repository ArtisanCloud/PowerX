package plugin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	pluginservice "github.com/ArtisanCloud/PowerX/internal/service/plugin"
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
		dtoRequest.ResponseSuccess(c, gin.H{"job": job})
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
		dtoRequest.ResponseSuccess(c, gin.H{"items": items})
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
		dtoRequest.ResponseSuccess(c, gin.H{"job": job})
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
		dtoRequest.ResponseSuccess(c, gin.H{"job": job})
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
		dtoRequest.ResponseSuccess(c, result)
	}
}
