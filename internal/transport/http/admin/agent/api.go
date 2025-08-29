package agent

// api/http/admin/agent/api.go

import "github.com/gin-gonic/gin"

func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup) {
	agentGroup := protectedGroup.Group("/agents")
	{
		// agentGroup.GET("/health", HealthHandler)
		agentGroup.GET("/status", AgentStatusHandler)
		agentGroup.POST("/chat", ChatHandler)
		agentGroup.POST("/stream", StreamChatHandler)
		agentGroup.POST("/intent/", AgentIntentHandler)
		agentGroup.POST("/intent/plan", AgentPlanPreviewHandler)
		// agentGroup.POST("/execute", ExecuteHandler)

		agentGroup.GET("/providers", listProviders)
		agentGroup.GET("/models", listModels)

		agentGroup.POST("/settings/save", saveSettings)     // 先占位：以后接DB
		agentGroup.POST("/test/connection", testConnection) // 真连通测试
		agentGroup.POST("/test/call", testQuickCall)        // 试跑一下
	}
}
