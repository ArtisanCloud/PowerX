package agent

// api/http/admin/agent/api.go

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	settingH := NewAgentSettingHandler(deps)
	agentH := NewAgentHandler(deps)
	chatH := NewAgentChatHandler(deps)
	agentGroup := protectedGroup.Group("/agents")
	{

		// agentGroup.GET("/health", HealthHandler)
		agentGroup.GET("/status", agentH.Status)
		agentGroup.POST("/intent/", agentH.Intent)
		agentGroup.POST("/intent/plan", agentH.PlanPreview)
		// agentGroup.POST("/execute", ExecuteHandler)
		agentGroup.POST("/stream", chatH.StreamChat)
		agentGroup.POST("/chat", chatH.Chat)

		agentGroup.GET("/providers", settingH.listProviders)
		agentGroup.GET("/models", settingH.listModels)

		agentGroup.POST("/settings/save", settingH.saveSettings)     // 先占位：以后接DB
		agentGroup.POST("/test/connection", settingH.testConnection) // 真连通测试
		agentGroup.POST("/test/call", settingH.testQuickCall)        // 试跑一下
	}
}
