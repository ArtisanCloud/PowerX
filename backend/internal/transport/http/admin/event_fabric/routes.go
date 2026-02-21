package eventfabric

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	workers "github.com/ArtisanCloud/PowerX/internal/app/shared/workers"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	adminnotifications "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/notifications"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterAPIRoutes 注册事件骨干相关 Admin API。
func RegisterAPIRoutes(_ *gin.RouterGroup, protected *gin.RouterGroup, deps *shared.Deps) {
	if deps == nil {
		return
	}

	// 兼容两套路由：
	// 1) 历史/插件侧：/event-fabric/*（可选签名校验）
	// 2) Web Admin：/admin/event-fabric/*（仅走 Bearer 鉴权 + Root 可见菜单）
	group := protected.Group("/event-fabric")
	if deps.EventFabric != nil && deps.EventFabric.Security != nil {
		group.Use(deps.EventFabric.Security.GinMiddleware())
	}
	adminGroup := protected.Group("/admin/event-fabric")

	if deps.EventFabric != nil && deps.EventFabric.Directory != nil {
		dirHandler := NewAdminDirectoryHandler(AdminDirectoryHandlerOptions{Service: deps.EventFabric.Directory})
		group.POST("/topics", dirHandler.CreateTopic)
		group.GET("/topics", dirHandler.ListTopics)
		group.PATCH("/topics/:topic_id/lifecycle", dirHandler.UpdateLifecycle)

		// Web Admin：Topic 管理（CRUD）+ 列表筛选
		adminGroup.POST("/topics", dirHandler.CreateTopic)
		adminGroup.GET("/topics", dirHandler.ListTopics)
		adminGroup.PATCH("/topics/:topic_id/lifecycle", dirHandler.UpdateLifecycle)
	}

	if deps.EventFabric != nil && deps.EventFabric.ACL != nil && deps.EventFabric.Directory != nil {
		aclHandler := NewAdminACLHandler(AdminACLHandlerOptions{
			Service:   deps.EventFabric.ACL,
			Directory: deps.EventFabric.Directory,
		})
		group.POST("/acl", aclHandler.UpsertBindings)
		group.GET("/acl", aclHandler.ListBindings)
		adminGroup.POST("/acl", aclHandler.UpsertBindings)
		adminGroup.GET("/acl", aclHandler.ListBindings)
		adminGroup.GET("/acl/topic-matrix", aclHandler.ListTopicRoleMatrix)
		adminGroup.GET("/acl/principal-matrix", aclHandler.ListPrincipalTopicMatrix)
	}

	if deps.EventFabric != nil && deps.EventFabric.Delivery != nil {
		deliveryHandler := NewAdminDeliveryHandler(AdminDeliveryHandlerOptions{Service: deps.EventFabric.Delivery})
		group.POST("/events:publish", deliveryHandler.PublishEvent)
	}

	overviewHandler := NewAdminOverviewHandler(AdminOverviewHandlerOptions{
		DB: deps.DB,
		Directory: func() *directory.DirectoryService {
			if deps.EventFabric != nil {
				return deps.EventFabric.Directory
			}
			return nil
		}(),
		Redis: func() *redis.Client {
			if deps.EventFabric != nil {
				return deps.EventFabric.RedisClient
			}
			return nil
		}(),
		Enabled: deps.EventFabric != nil,
	})
	group.GET("/overview", overviewHandler.GetOverview)
	adminGroup.GET("/overview", overviewHandler.GetOverview)
	adminGroup.GET("/task-queue/stats", overviewHandler.GetTaskQueueStats)
	adminGroup.GET("/task-queue/messages", overviewHandler.GetTaskQueueMessages)

	if deps.EventFabric != nil && deps.EventFabric.Authorization != nil && deps.EventFabric.Authorization.Service != nil {
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

	if deps.EventFabric != nil && deps.EventFabric.DLQ != nil {
		dlqHandler := NewAdminDLQHandler(AdminDLQHandlerOptions{Service: deps.EventFabric.DLQ})
		group.GET("/dlq/messages", dlqHandler.ListMessages)
		group.POST("/dlq/messages:replay", dlqHandler.ReplayMessages)
		group.DELETE("/dlq/messages", dlqHandler.PurgeMessages)

		// Web Admin 只暴露 DLQ 列表 + replay（不暴露 purge）
		adminGroup.GET("/dlq/messages", dlqHandler.ListMessages)
		adminGroup.POST("/dlq/messages:replay", dlqHandler.ReplayMessages)
	}

	if deps.EventFabric != nil && deps.EventFabric.Replay != nil {
		replayHandler := NewAdminReplayHandler(AdminReplayHandlerOptions{Service: deps.EventFabric.Replay})
		group.POST("/replay/tasks", replayHandler.CreateTask)
		group.GET("/replay/tasks/:task_id", replayHandler.GetTask)
		group.POST("/replay/tasks/:task_id/cancel", replayHandler.CancelTask)

		adminGroup.POST("/replay/tasks", replayHandler.CreateTask)
		adminGroup.GET("/replay/tasks/:task_id", replayHandler.GetTask)
		adminGroup.POST("/replay/tasks/:task_id/cancel", replayHandler.CancelTask)

		// 统一调试入口（保留旧接口兼容）
		adminGroup.POST("/debug/tasks/replay", replayHandler.CreateTask)
		adminGroup.GET("/debug/tasks/replay/:task_id", replayHandler.GetTask)
		adminGroup.POST("/debug/tasks/replay/:task_id/cancel", replayHandler.CancelTask)
	}

	if deps.EventFabric != nil && deps.EventFabric.TaskDriver != nil {
		notificationsHandler := adminnotifications.NewHandler(deps)
		adminGroup.POST("/debug/tasks/pipeline", notificationsHandler.PushTestNotificationQueue)
	}

	if deps.EventFabric != nil {
		var retryWorker *workers.EventFabricRetryWorker
		var authWorker *workers.EventFabricAuthorizationTimeoutTaskWorker
		retryWorker = deps.EventFabric.RetryWorker
		if deps.EventFabric.Authorization != nil {
			authWorker = deps.EventFabric.Authorization.TimeoutTaskWorker
		}

		cronHandler := NewAdminCronHandler(AdminCronHandlerOptions{
			RetryWorker:         retryWorker,
			AuthorizationWorker: authWorker,
		})
		adminGroup.GET("/cron/jobs", cronHandler.ListJobs)
		adminGroup.POST("/cron/jobs/:job_id/run-now", cronHandler.RunNow)
		adminGroup.POST("/cron/jobs/:job_id/pause", cronHandler.PauseJob)
		adminGroup.POST("/cron/jobs/:job_id/resume", cronHandler.ResumeJob)
	}
}
