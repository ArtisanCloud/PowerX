package http

import (
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// HealthHandler 健康检查处理器
func HealthHandler(c *gin.Context) {
	dto.ResponseSuccess(c, gin.H{
		"status":  "ok",
		"service": "CoreX",
	})
}
