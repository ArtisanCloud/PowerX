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

	}
	agentAdminGroup := protectedGroup.Group("/admin/agents")
	{
		agentAdminGroup.GET("/providers", settingH.listProviders)
		agentAdminGroup.GET("/models", settingH.listModels)

		agentAdminGroup.POST("/settings/save", settingH.saveSettings)     // 先占位：以后接DB
		agentAdminGroup.POST("/test/connection", settingH.testConnection) // 真连通测试
		agentAdminGroup.POST("/test/call", settingH.testQuickCall)        // 试跑一下

		agentAdminGroup.GET("/settings/profiles", settingH.listProfiles)
		agentAdminGroup.GET("/settings/credentials", settingH.listCredentials)

		agentAdminGroup.GET("/settings/active", settingH.getActiveProfile)
		agentAdminGroup.POST("/settings/active", settingH.setActiveProfile)

	}
}
