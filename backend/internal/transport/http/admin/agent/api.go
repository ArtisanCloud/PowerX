package agent

// api/http/admin/agent/api.go

import (
	"context"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	adminauthz "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/authz"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	coreiam "github.com/ArtisanCloud/PowerX/pkg/corex/iam"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func RegisterAPIRoutes(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	settingH := NewAgentSettingHandler(deps)
	agentH := NewAgentHandler(deps)
	chatH := NewAgentChatHandler(deps)
	shareH := NewShareHandler(deps)
	teamH := NewTeamHandler(deps)
	authzH := NewAgentAuthzHandler(deps)

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
		agentGroup.POST("/sessions/:id/capability-approvals/:approval_uuid/resume", chatH.ResumeApprovedCapability)
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
		agentAdminGroup.GET("/settings/skills/source-policy", settingH.getSkillSourcePolicy)
		agentAdminGroup.PUT("/settings/skills/source-policy", settingH.setSkillSourcePolicy)
		agentAdminGroup.GET("/settings/context-optimizer/active", settingH.getContextOptimizerActive)
		agentAdminGroup.POST("/settings/context-optimizer/drafts", settingH.saveContextOptimizerDraft)
		agentAdminGroup.GET("/settings/context-optimizer/versions", settingH.listContextOptimizerVersions)
		agentAdminGroup.POST("/settings/context-optimizer/publish", settingH.publishContextOptimizer)
		agentAdminGroup.POST("/settings/context-optimizer/rollback", settingH.rollbackContextOptimizer)

		// 智能体 CRUD
		agentAdminGroup.POST("", adminauthz.AdminOrPluginRegistrySyncMiddleware(deps, adminauthz.ScopePluginAgentRegistrySync), agentH.CreateAgent)
		agentAdminGroup.GET("", agentH.ListAgents)
		agentAdminGroup.GET("/grantable-capabilities", authzH.ListGrantableCapabilities)
		agentAdminGroup.GET("/capability-approvals", requireAgentCapabilityApprovalAdmin(deps), chatH.ListPendingCapabilityApprovals)
		agentAdminGroup.POST("/capability-approvals/:approval_uuid/approve", requireAgentCapabilityApprovalAdmin(deps), chatH.ApproveCapabilityApproval)
		agentAdminGroup.POST("/capability-approvals/:approval_uuid/reject", requireAgentCapabilityApprovalAdmin(deps), chatH.RejectCapabilityApproval)
		agentAdminGroup.GET("/runtime-replays/:run_uuid/snapshots/:snapshot_uuid/revisions/:revision_uuid/validate", requireAgentCapabilityApprovalAdmin(deps), chatH.ValidateRuntimeReplay)
		agentAdminGroup.GET("/:uuid", agentH.GetAgent)
		agentAdminGroup.PATCH("/:uuid", adminauthz.AdminOrPluginRegistrySyncMiddleware(deps, adminauthz.ScopePluginAgentRegistrySync), agentH.UpdateAgent)
		agentAdminGroup.POST("/:uuid/enable", agentH.EnableAgent)
		agentAdminGroup.POST("/:uuid/disable", agentH.DisableAgent)
		agentAdminGroup.GET("/:uuid/grants", authzH.ListAgentGrants)
		agentAdminGroup.PATCH("/:uuid/grants", authzH.PatchAgentGrants)
		agentAdminGroup.PUT("/:uuid/grants", authzH.ReplaceAgentGrants)
		agentAdminGroup.GET("/:uuid/access-grants", authzH.ListAgentAccessGrants)
		agentAdminGroup.PATCH("/:uuid/access-grants", authzH.PatchAgentAccessGrants)
		agentAdminGroup.GET("/:uuid/my-effective-permissions", authzH.MyEffectivePermissions)
		agentAdminGroup.GET("/:uuid/effective-permissions", authzH.EffectivePermissions)

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

		teamGroup := agentAdminGroup.Group("/teams")
		{
			teamGroup.POST("", teamH.CreateTeam)
			teamGroup.GET("", teamH.ListTeams)
			teamGroup.PATCH("/:teamId", teamH.UpdateTeam)
			teamGroup.PATCH("/:teamId/status", teamH.SetTeamStatus)
			teamGroup.DELETE("/:teamId", teamH.DeleteTeam)
			teamGroup.GET("/:teamId/members", teamH.ListTeamMembers)
			teamGroup.PUT("/:teamId/members", teamH.UpsertTeamMember)
			teamGroup.DELETE("/:teamId/members/:childAgentId", teamH.DeleteTeamMember)
		}
	}

	// Admin AI OpenAPI mapping
	adminAIGroup := protectedGroup.Group("/admin/ai")
	{
		adminAIGroup.GET("/llm/models", settingH.listOpenAILLMModels)
	}
}

func requireAgentCapabilityApprovalAdmin(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if reqctx.IsRoot(c.Request.Context()) || hasAgentCapabilityApprovalAdminRole(c.Request.Context(), deps) {
			c.Next()
			return
		}
		dto.ResponseError(c, http.StatusForbidden, "agent.capability_approval_admin_required", nil)
		c.Abort()
	}
}

func hasAgentCapabilityApprovalAdminRole(ctx context.Context, deps *shared.Deps) bool {
	if deps == nil || deps.DB == nil {
		return false
	}
	tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	memberID := reqctx.GetMemberID(ctx)
	if tenantUUID == "" || memberID == 0 {
		return false
	}
	roles, err := iamrepo.NewRoleBindingRepository(deps.DB).ListRolesByMember(ctx, tenantUUID, memberID)
	if err != nil {
		return false
	}
	for _, role := range roles {
		if strings.TrimSpace(role.TenantUUID) != tenantUUID || strings.TrimSpace(strings.ToLower(role.Scope)) != string(coreiam.RoleScopeTenant) {
			continue
		}
		if role.Code == coreiam.CodeRoleOwner || role.Code == coreiam.CodeRoleAdmin {
			return true
		}
	}
	return false
}
