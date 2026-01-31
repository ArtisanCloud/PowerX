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
	shareH := NewShareHandler(deps)

	sessionH := NewAgentSessionHandler(deps)
	tenantFormH := NewTenantAgentFormHandler(deps)

	agentGroup := protectedGroup.Group("/agents")
	{

		// agentGroup.GET("/health", HealthHandler)
		agentGroup.GET("/status", agentH.Status)
		agentGroup.POST("/intent/", agentH.Intent)
		agentGroup.POST("/intent/plan", agentH.PlanPreview)
		// agentGroup.POST("/execute", ExecuteHandler)
		agentGroup.GET("/stream/mock", chatH.SimulateSSE) // 新增：标准 GET SSE（方便前端用 EventSource）
		agentGroup.GET("/stream/sse", chatH.StreamSSE)    // 新增：标准 GET SSE（方便前端用 EventSource）

		// 新增：POST 普通 Chat（非流）
		agentGroup.POST("/invoke", chatH.Invoke)
		agentGroup.POST("/sessions/:id/invoke", chatH.InvokeSession)
		agentGroup.GET("/sessions/:id/stream/sse", chatH.StreamSessionSSE)

		agentGroup.POST("/sessions", sessionH.CreateSession)
		agentGroup.GET("/sessions", sessionH.ListSessions)
		agentGroup.GET("/sessions/:id", sessionH.GetSession)
		agentGroup.PATCH("/sessions/:id", sessionH.UpdateSession)
		agentGroup.POST("/sessions/:id/archive", sessionH.ArchiveSession)
		agentGroup.DELETE("/sessions/:id", sessionH.DeleteSession)

		agentGroup.GET("/sessions/:id/messages", sessionH.ListMessages)

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

		// 智能体 CRUD
		agentAdminGroup.POST("", agentH.CreateAgent)
		agentAdminGroup.GET("", agentH.ListAgents)
		agentAdminGroup.GET("/:uuid", agentH.GetAgent)
		agentAdminGroup.PATCH("/:uuid", agentH.UpdateAgent)
		agentAdminGroup.POST("/:uuid/enable", agentH.EnableAgent)
		agentAdminGroup.POST("/:uuid/disable", agentH.DisableAgent)

		agentAdminGroup.POST("/:uuid/shares", shareH.CreateShare)
		agentAdminGroup.POST("/shares/:share_id/revoke", shareH.RevokeShare)
		agentAdminGroup.DELETE("/:uuid", agentH.DeleteAgent)

		// 智能体 AI 配置
		agentAdminGroup.GET("/:uuid/ai-setting", agentH.GetAgentAISetting)
		agentAdminGroup.PUT("/:uuid/ai-setting", agentH.UpsertAgentAISetting)
		agentAdminGroup.DELETE("/:uuid/ai-setting", agentH.DeleteAgentAISetting)
		agentAdminGroup.POST("/:uuid/health-check", agentH.AgentHealthCheck)

		tenantFormsGroup := agentAdminGroup.Group("/tenant/forms")
		{
			tenantFormsGroup.POST("", tenantFormH.SubmitTenantForm)
			tenantFormsGroup.GET("", tenantFormH.ListTenantForms)
			tenantFormsGroup.GET("/:form_id", tenantFormH.GetTenantForm)
			tenantFormsGroup.POST("/:form_id/approve", tenantFormH.ApproveTenantForm)
			tenantFormsGroup.POST("/:form_id/reject", tenantFormH.RejectTenantForm)
		}
	}
}
