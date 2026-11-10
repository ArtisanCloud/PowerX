package integration_gateway

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// 保留业务状态、稳定错误码和管理诊断明细，避免配置冲突被包装为无明细的 500。
func respondAPIKeyPermissionError(c *gin.Context, err error) {
	details := dto.DetailsOf(err)
	if traceID := reqctx.GetTraceID(c.Request.Context()); traceID != "" {
		if details == nil {
			details = map[string]interface{}{}
		}
		details["trace_id"] = traceID
	}
	dto.ResponseErrorWithDetails(c, dto.StatusCode(err), dto.MessageOf(err), err, details)
}
