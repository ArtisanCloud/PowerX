package agent

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
	}
}
