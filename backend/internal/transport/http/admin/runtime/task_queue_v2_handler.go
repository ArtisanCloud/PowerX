package runtime

import (
	"encoding/json"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	tq "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func taskQueueV2Handler(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps == nil || deps.EventFabric == nil {
			dto.ResponseError(c, 503, "TASK_QUEUE_UNAVAILABLE", nil)
			return
		}
		var body json.RawMessage
		if c.ShouldBindJSON(&body) != nil {
			dto.ResponseError(c, 400, "TASK_QUEUE_INVALID_ARGUMENT", nil)
			return
		}
		in, err := tq.DecodeRuntimeRequest(body)
		if err != nil {
			dto.ResponseError(c, 400, "TASK_QUEUE_INVALID_ARGUMENT", err)
			return
		}
		result, err := tq.NewRuntimeInvoker(deps.DB, deps.EventFabric.StreamTasks).ApplyAdmin(c.Request.Context(), in)
		if err != nil {
			dto.ResponseError(c, dto.StatusCode(err), dto.MessageOf(err), err)
			return
		}
		dto.ResponseSuccess(c, result)
	}
}
