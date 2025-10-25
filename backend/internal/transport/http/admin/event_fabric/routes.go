package eventfabric

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/gin-gonic/gin"
)

// RegisterAPIRoutes 注册事件骨干相关 Admin API。
func RegisterAPIRoutes(_ *gin.RouterGroup, protected *gin.RouterGroup, deps *shared.Deps) {
	if deps == nil || deps.EventFabric == nil {
		return
	}

	group := protected.Group("/event-fabric")
	if deps.EventFabric.Security != nil {
		group.Use(deps.EventFabric.Security.GinMiddleware())
	}

	if deps.EventFabric.Directory != nil {
		dirHandler := NewAdminDirectoryHandler(AdminDirectoryHandlerOptions{Service: deps.EventFabric.Directory})
		group.POST("/topics", dirHandler.CreateTopic)
		group.GET("/topics", dirHandler.ListTopics)
		group.PATCH("/topics/:topic_id/lifecycle", dirHandler.UpdateLifecycle)
	}

	if deps.EventFabric.ACL != nil && deps.EventFabric.Directory != nil {
		aclHandler := NewAdminACLHandler(AdminACLHandlerOptions{
			Service:   deps.EventFabric.ACL,
			Directory: deps.EventFabric.Directory,
		})
		group.POST("/acl", aclHandler.UpsertBindings)
		group.GET("/acl", aclHandler.ListBindings)
	}

	if deps.EventFabric.Delivery != nil {
		deliveryHandler := NewAdminDeliveryHandler(AdminDeliveryHandlerOptions{Service: deps.EventFabric.Delivery})
		group.POST("/events:publish", deliveryHandler.PublishEvent)
	}

	if deps.EventFabric.Authorization != nil && deps.EventFabric.Authorization.Service != nil {
		authHandler := NewAuthorizationHandler(AuthorizationHandlerOptions{
			Service:   deps.EventFabric.Authorization.Service,
			Templates: deps.EventFabric.Authorization.Templates,
			Reporting: deps.EventFabric.Authorization.Reporting,
		})
		group.POST("/capabilities", authHandler.CreateCapability)
		group.PATCH("/capabilities/:capabilityId", authHandler.UpdateCapability)
		group.POST("/grants", authHandler.CreateGrant)
		group.GET("/grants/:grantId", authHandler.GetGrant)
		group.PATCH("/grants/:grantId", authHandler.UpdateGrant)
		group.POST("/grants/:grantId/revoke", authHandler.RevokeGrant)
		group.POST("/grants/cache:invalidate", authHandler.InvalidateGrantCache)
		group.POST("/challenges/:ticketId/decision", authHandler.DecideChallenge)
		group.GET("/audit/authorization", authHandler.ListAuthorizationAudit)
		group.GET("/grant-templates", authHandler.ListTemplates)
		group.POST("/grant-templates", authHandler.CreateTemplate)
		group.PATCH("/grant-templates/:templateId", authHandler.UpdateTemplate)
		group.DELETE("/grant-templates/:templateId", authHandler.DeleteTemplate)
		group.POST("/grant-templates/:templateId/apply", authHandler.ApplyTemplate)
	}

	if deps.EventFabric.DLQ != nil {
		dlqHandler := NewAdminDLQHandler(AdminDLQHandlerOptions{Service: deps.EventFabric.DLQ})
		group.GET("/dlq/messages", dlqHandler.ListMessages)
		group.POST("/dlq/messages:replay", dlqHandler.ReplayMessages)
		group.DELETE("/dlq/messages", dlqHandler.PurgeMessages)
	}

	if deps.EventFabric.Replay != nil {
		replayHandler := NewAdminReplayHandler(AdminReplayHandlerOptions{Service: deps.EventFabric.Replay})
		group.POST("/replay/tasks", replayHandler.CreateTask)
		group.GET("/replay/tasks/:task_id", replayHandler.GetTask)
		group.POST("/replay/tasks/:task_id/cancel", replayHandler.CancelTask)
	}
}
