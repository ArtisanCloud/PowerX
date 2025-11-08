// internal/bootstrap/app.go
package bootstrap

import (
	"context"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/bootstrap"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/catalog"
	"github.com/ArtisanCloud/PowerX/internal/service/auth"
	security "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/security"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	pkgauth "github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"log"
	"strings"
	"time"
)

func BootstrapApp(ctx context.Context, cfg *config.Config) (*shared.Deps, error) {

	// 初始化全局 Logger
	logger.InitGlobalLogger(&cfg.LogConfig)
	logger.Info(ctx, "🚀 全局 Logger 初始化成功")

	// 读取 Wrap 密钥
	if _, err := cfg.Server.ParseKey(); err != nil {
		log.Fatalf("读取 server.secret_key 失败: %v", err)
	} else {
		logger.Info(ctx, "Wrap 密钥已设置到全局")
	}

	// 将 JWT Secret 注入全局，供插件网关签发与验签复用
	if len(cfg.Auth.JWTSecret) > 0 {
		pkgauth.SetJWTSecret([]byte(cfg.Auth.JWTSecret))
	}

	// 初始化数据库连接（GORM）
	db, err := database.GetDB(&cfg.Database)
	if err != nil {
		logger.ErrorF(ctx, "初始化数据库失败: %v", err)
		return nil, err
	}

	// 初始化缓存
	_, err = cache.InitCache(&cfg.Cache)
	if err != nil {
		logger.ErrorF(ctx, "初始化缓存失败: %s", err.Error())
		return nil, err
	}

	// 加载 AI Catalog 配置
	if err := catalog.InitFromAppConfig(cfg.AI.Catalog, nil); err != nil {
		return nil, err
	}
	n := len(catalog.GetGlobalAIRegister().Providers("llm"))
	logger.InfoF(ctx, "[catalog] loaded providers: %d", n)

	// 初始化智能体工具（Agent Tools）
	err = bootstrap.InitAgentTools(ctx, &cfg.Agent, db)
	if err != nil {
		logger.ErrorF(ctx, "初始化工具失败: %s", err.Error())
		return nil, err
	}

	// 初始化事件总线（EventBus）
	err = event_bus.InitEventBus()
	if err != nil {
		logger.ErrorF(ctx, "初始化事件总线失败: %s", err.Error())
	}

	// 构建应用依赖（认证 / 审计等）
	accessTTL, _ := time.ParseDuration(cfg.Auth.AccessTTLStr)
	refreshTTL, _ := time.ParseDuration(cfg.Auth.RefreshTTLStr)
	localTokenSecret := strings.TrimSpace(cfg.Storage.Local.UploadTokenSecret)
	if cfg.Storage.Local.EnableUploadEndpoint && localTokenSecret == "" {
		logger.WarnF(ctx, "storage.local.enable_upload_endpoint 已启用，但未配置 upload_token_secret，上传端点将在路由层被禁用")
	}
	maxUploadSize := cfg.Storage.Local.MaxUploadSizeBytes
	if maxUploadSize < 0 {
		maxUploadSize = 0
	}

	opts := &shared.DepsOptions{
		AuthUser: auth.AuthOptions{
			JWTSecret:  []byte(cfg.Auth.JWTSecret),
			Issuer:     cfg.Auth.Issuer,
			Audience:   cfg.Auth.AudienceUser,
			Platforms:  cfg.Auth.Platforms,
			AccessTTL:  accessTTL,
			RefreshTTL: refreshTTL,
		},
		AuthCustomer: auth.AuthOptions{
			JWTSecret:  []byte(cfg.Auth.JWTSecret),
			Issuer:     cfg.Auth.Issuer,
			Audience:   cfg.Auth.AudienceCustomer,
			Platforms:  cfg.Auth.Platforms,
			AccessTTL:  accessTTL,
			RefreshTTL: refreshTTL,
		},
		Audit: auditsvc.AuditOptions{
			BatchSize: 200, BatchWait: 150 * time.Millisecond, MaxPayloadSize: 16 * 1024,
		},
		Storage: mediasvc.StorageOptions{
			DefaultDriver: cfg.Storage.DefaultDriver,
			TTLSeconds:    cfg.Storage.TTLSeconds,
			Local: mediasvc.StorageLocalOptions{
				BasePath:             cfg.Storage.Local.BasePath,
				PublicBaseURL:        cfg.Storage.Local.PublicBaseURL,
				EnableUploadEndpoint: cfg.Storage.Local.EnableUploadEndpoint,
				UploadTokenSecret:    localTokenSecret,
				MaxUploadSizeBytes:   maxUploadSize,
			},
			S3: mediasvc.StorageS3Options{
				Endpoint:        cfg.Storage.S3.Endpoint,
				Region:          cfg.Storage.S3.Region,
				AccessKey:       cfg.Storage.S3.AccessKey,
				SecretKey:       cfg.Storage.S3.SecretKey,
				SessionToken:    cfg.Storage.S3.SessionToken,
				Bucket:          cfg.Storage.S3.Bucket,
				UseSSL:          cfg.Storage.S3.UseSSL,
				ForcePathStyle:  cfg.Storage.S3.ForcePathStyle,
				ExternalDomain:  cfg.Storage.S3.ExternalDomain,
				PresignEndpoint: cfg.Storage.S3.PresignEndpoint,
			},
		},
		EventFabric: shared.EventFabricOptions{
			AckTimeoutSeconds: cfg.EventFabric.AckTimeoutSeconds,
			DefaultMaxRetry:   cfg.EventFabric.DefaultMaxRetry,
			RedisAddr:         cfg.EventFabric.RedisAddr,
			RedisPassword:     cfg.EventFabric.RedisPassword,
			RedisDB:           cfg.EventFabric.RedisDB,
			RetryKeyPrefix:    cfg.EventFabric.RetryKeyPrefix,
			ReplayKeyPrefix:   cfg.EventFabric.ReplayKeyPrefix,
			SchedulerInterval: cfg.EventFabric.SchedulerInterval,
			Security: security.Config{
				RequireTLS:           cfg.EventFabric.Security.RequireTLS,
				SignatureSecret:      cfg.EventFabric.Security.SignatureSecret,
				SignatureHeader:      cfg.EventFabric.Security.SignatureHeader,
				TimestampHeader:      cfg.EventFabric.Security.TimestampHeader,
				SignatureKeyID:       cfg.EventFabric.Security.SignatureKeyID,
				AllowedClockSkew:     time.Duration(cfg.EventFabric.Security.AllowedClockSkewSeconds) * time.Second,
				ProtectedGRPCService: "/corex.event_fabric.v1.",
				Sandbox: security.SandboxConfig{
					Enforce:              cfg.EventFabric.Security.Sandbox.Enforce,
					AllowedOutboundHosts: cfg.EventFabric.Security.Sandbox.AllowedOutboundHosts,
					BlockedHTTPPaths:     cfg.EventFabric.Security.Sandbox.BlockedHTTPPaths,
					BlockedGRPCMethods:   cfg.EventFabric.Security.Sandbox.BlockedGRPCMethods,
					ForbiddenHeaders:     cfg.EventFabric.Security.Sandbox.ForbiddenHeaders,
				},
			},
			Authorization: shared.EventFabricAuthorizationOptions{
				CacheTTLSeconds:             cfg.EventFabric.Authorization.CacheTTLSeconds,
				LocalCacheTTLSeconds:        cfg.EventFabric.Authorization.LocalCacheTTLSeconds,
				RedisAddr:                   cfg.EventFabric.Authorization.RedisAddr,
				RedisPassword:               cfg.EventFabric.Authorization.RedisPassword,
				RedisDB:                     cfg.EventFabric.Authorization.RedisDB,
				CacheInvalidateChannel:      cfg.EventFabric.Authorization.CacheInvalidateChannel,
				ChallengeSLASeconds:         cfg.EventFabric.Authorization.ChallengeSLASeconds,
				ChallengeTopic:              cfg.EventFabric.Authorization.ChallengeTopic,
				ChallengeConsumerGroup:      cfg.EventFabric.Authorization.ChallengeConsumerGroup,
				AlertTopic:                  cfg.EventFabric.Authorization.AlertTopic,
				RateLimitPrefix:             cfg.EventFabric.Authorization.RateLimitPrefix,
				TimeoutSweepIntervalSeconds: cfg.EventFabric.Authorization.TimeoutSweepIntervalSeconds,
				AuditRetentionDays:          cfg.EventFabric.Authorization.AuditRetentionDays,
				AuditArchiveBucket:          cfg.EventFabric.Authorization.AuditArchiveBucket,
				AuditArchivePrefix:          cfg.EventFabric.Authorization.AuditArchivePrefix,
				Secrets: shared.EventFabricAuthorizationSecretsOptions{
					Provider:                cfg.EventFabric.Authorization.Secrets.Provider,
					KeyID:                   cfg.EventFabric.Authorization.Secrets.KeyID,
					RotationIntervalSeconds: cfg.EventFabric.Authorization.Secrets.RotationIntervalSeconds,
					CacheTTLSeconds:         cfg.EventFabric.Authorization.Secrets.CacheTTLSeconds,
				},
			},
		},
		IntegrationGateway: shared.IntegrationGatewayOptions{
			RateLimitPrefix: cfg.IntegrationGateway.RateLimitPrefix,
			RedisAddr:       cfg.IntegrationGateway.RedisAddr,
			RedisPassword:   cfg.IntegrationGateway.RedisPassword,
			RedisDB:         cfg.IntegrationGateway.RedisDB,
			DefaultRateLimit: shared.IntegrationGatewayRateLimitOptions{
				Limit:         cfg.IntegrationGateway.DefaultRateLimit.Limit,
				Burst:         cfg.IntegrationGateway.DefaultRateLimit.Burst,
				WindowSeconds: cfg.IntegrationGateway.DefaultRateLimit.WindowSeconds,
				Scope:         cfg.IntegrationGateway.DefaultRateLimit.Scope,
			},
			EventTopics: shared.IntegrationGatewayEventTopicsOptions{
				Created:             cfg.IntegrationGateway.EventTopics.Created,
				Updated:             cfg.IntegrationGateway.EventTopics.Updated,
				InvocationSucceeded: cfg.IntegrationGateway.EventTopics.InvocationSucceeded,
				InvocationFailed:    cfg.IntegrationGateway.EventTopics.InvocationFailed,
			},
		},
		AgentLifecycle: shared.AgentLifecycleOptions{
			RedisAddr:                cfg.AgentLifecycle.RedisAddr,
			RedisPassword:            cfg.AgentLifecycle.RedisPassword,
			RedisDB:                  cfg.AgentLifecycle.RedisDB,
			CapacityKeyPrefix:        cfg.AgentLifecycle.CapacityKeyPrefix,
			HealthKeyPrefix:          cfg.AgentLifecycle.HealthKeyPrefix,
			DefaultCapacityInstances: cfg.AgentLifecycle.DefaultCapacityInstances,
			EventTopics: shared.AgentLifecycleEventTopicsOptions{
				LifecyclePrefix: cfg.AgentLifecycle.EventTopics.LifecyclePrefix,
				HealthPrefix:    cfg.AgentLifecycle.EventTopics.HealthPrefix,
			},
			Notifications: shared.AgentLifecycleNotificationOptions{
				IMWebhook:        cfg.AgentLifecycle.Notifications.IMWebhook,
				RetryInterval:    time.Duration(cfg.AgentLifecycle.Notifications.RetryIntervalSec) * time.Second,
				RetryMaxAttempts: cfg.AgentLifecycle.Notifications.RetryMaxAttempts,
				HTTPTimeout:      time.Duration(cfg.AgentLifecycle.Notifications.HTTPTimeoutSec) * time.Second,
			},
		},
		PluginRelease: shared.PluginReleaseOptions{
			FeatureFlags: shared.PluginReleaseFeatureFlagsOptions{
				EnableLocalInstall:        cfg.PluginRelease.FeatureFlags.EnableLocalInstall,
				EnablePipelineDeployment:  cfg.PluginRelease.FeatureFlags.EnablePipelineDeployment,
				EnableOfflineDistribution: cfg.PluginRelease.FeatureFlags.EnableOfflineDistribution,
			},
			LocalInstall: shared.PluginReleaseLocalInstallOptions{
				SessionTTL:        time.Duration(cfg.PluginRelease.LocalInstall.SessionTTLMinutes) * time.Minute,
				MaxArtifactSizeMB: cfg.PluginRelease.LocalInstall.MaxArtifactSizeMB,
			},
			Pipeline: shared.PluginReleasePipelineOptions{
				ApprovalSLA:           time.Duration(cfg.PluginRelease.Pipeline.ApprovalSLAHours) * time.Hour,
				MaxParallelReleases:   cfg.PluginRelease.Pipeline.MaxParallelReleases,
				DefaultRollbackNotice: time.Duration(cfg.PluginRelease.Pipeline.DefaultRollbackNotice) * time.Minute,
			},
			Canary: shared.PluginReleaseCanaryOptions{
				RollbackTimeout:  time.Duration(cfg.PluginRelease.Canary.RollbackTimeoutSeconds) * time.Second,
				DefaultBatchSize: cfg.PluginRelease.Canary.DefaultBatchSize,
				MaxBatches:       cfg.PluginRelease.Canary.MaxBatches,
			},
			Distribution: shared.PluginReleaseDistributionOptions{
				OfflineBucket:       cfg.PluginRelease.Distribution.OfflineBucket,
				OfflinePrefix:       cfg.PluginRelease.Distribution.OfflinePrefix,
				EscalationThreshold: cfg.PluginRelease.Distribution.EscalationThreshold,
				ArtifactRetention:   time.Duration(cfg.PluginRelease.Distribution.ArtifactRetentionDays) * 24 * time.Hour,
			},
			Observability: shared.PluginReleaseObservabilityOptions{
				DashboardUID:    cfg.PluginRelease.Observability.DashboardUID,
				AlertRulePrefix: cfg.PluginRelease.Observability.AlertRulePrefix,
				KPITargets: shared.PluginReleaseKPITargetsOptions{
					CanRollbackWithin: time.Duration(cfg.PluginRelease.Observability.KPITargets.CanRollbackWithinSeconds) * time.Second,
					HotloadLatencyP95: time.Duration(cfg.PluginRelease.Observability.KPITargets.HotloadLatencyP95Ms) * time.Millisecond,
				},
			},
		},
		PluginBootstrap: shared.PluginBootstrapOptions{
			TemplatesPath:   cfg.PluginBootstrap.TemplatesIndex,
			DefaultTemplate: cfg.PluginBootstrap.DefaultTemplate,
			AllowHosts:      cfg.PluginBootstrap.AllowlistedHosts,
		},
	}

	deps := shared.NewDeps(db, opts)

	return deps, nil
}
