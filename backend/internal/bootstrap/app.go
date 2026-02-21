// internal/bootstrap/app.go
package bootstrap

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

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
	milvuscfg "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore/milvus"
	pgvectorcfg "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore/pgvector"
	pineconecfg "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore/pinecone"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

func composePostgresDSNFromDB(driver string, host string, port int, user string, password string, database string, sslMode string, timezone string) string {
	if strings.TrimSpace(host) == "" {
		return ""
	}
	if strings.TrimSpace(sslMode) == "" {
		sslMode = "disable"
	}
	if strings.TrimSpace(timezone) == "" {
		timezone = "UTC"
	}
	if driver != "" && !strings.EqualFold(strings.TrimSpace(driver), "postgres") && !strings.EqualFold(strings.TrimSpace(driver), "pg") {
		return ""
	}
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		host, port, user, password, database, sslMode, timezone,
	)
}

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

	queueRedisAddr := strings.TrimSpace(cfg.Queue.Redis.Addr)
	if queueRedisAddr == "" {
		queueRedisAddr = fmt.Sprintf("%s:%d", strings.TrimSpace(cfg.Cache.Host), cfg.Cache.Port)
	}
	queueRedisPassword := strings.TrimSpace(cfg.Queue.Redis.Password)
	if queueRedisPassword == "" {
		queueRedisPassword = cfg.Cache.Password
	}
	queueRedisDB := cfg.Queue.Redis.DB

	globalSchedulerInterval := cfg.Scheduler.IntervalSeconds
	if globalSchedulerInterval <= 0 {
		globalSchedulerInterval = 5
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
	eventBusCfg := buildEventBusConfig(cfg, queueRedisAddr, queueRedisPassword, queueRedisDB)
	err = event_bus.InitEventBus(eventBusCfg)
	if err != nil {
		logger.ErrorF(ctx, "初始化事件总线失败: %s", err.Error())
	}

	// 构建应用依赖（认证 / 审计等）
	accessTTL, _ := time.ParseDuration(cfg.Auth.AccessTTLStr)
	refreshTTL, _ := time.ParseDuration(cfg.Auth.RefreshTTLStr)
	localTokenSecret := strings.TrimSpace(cfg.Storage.Local.UploadTokenSecret)
	if localTokenSecret == "" {
		logger.WarnF(ctx, "storage.local.upload_token_secret 未配置，本地上传端点将跳过 Token 校验，不建议在生产环境使用")
	}
	publicTokenSecret := strings.TrimSpace(cfg.Storage.Local.PublicTokenSecret)
	if publicTokenSecret == "" {
		// 兼容：未显式配置时，优先复用 upload_token_secret，其次回退到 JWTSecret（避免本地环境“复制下载链接”不可用）。
		if localTokenSecret != "" {
			publicTokenSecret = localTokenSecret
		} else if strings.TrimSpace(cfg.Auth.JWTSecret) != "" {
			publicTokenSecret = strings.TrimSpace(cfg.Auth.JWTSecret)
			logger.WarnF(ctx, "storage.local.public_token_secret 未配置，已回退使用 auth.jwt_secret 作为公开下载 token 密钥（建议在生产环境显式配置 public_token_secret）")
		}
	}
	maxUploadSize := cfg.Storage.Local.MaxUploadSizeBytes
	if maxUploadSize < 0 {
		maxUploadSize = 0
	}

	eventRedisAddr := strings.TrimSpace(cfg.Event.Fabric.RedisAddr)
	if eventRedisAddr == "" {
		eventRedisAddr = queueRedisAddr
	}
	eventRedisPassword := strings.TrimSpace(cfg.Event.Fabric.RedisPassword)
	if eventRedisPassword == "" {
		eventRedisPassword = queueRedisPassword
	}
	eventRedisDB := cfg.Event.Fabric.RedisDB
	if eventRedisDB == 0 {
		eventRedisDB = queueRedisDB
	}
	schedulerInterval := cfg.Event.Fabric.SchedulerInterval
	if schedulerInterval <= 0 {
		schedulerInterval = globalSchedulerInterval
	}
	authRedisAddr := strings.TrimSpace(cfg.Event.Fabric.Authorization.RedisAddr)
	if authRedisAddr == "" {
		authRedisAddr = eventRedisAddr
	}
	authRedisPassword := strings.TrimSpace(cfg.Event.Fabric.Authorization.RedisPassword)
	if authRedisPassword == "" {
		authRedisPassword = eventRedisPassword
	}
	authRedisDB := cfg.Event.Fabric.Authorization.RedisDB
	if authRedisDB == 0 {
		authRedisDB = eventRedisDB
	}

	knowledgeRedisAddr := strings.TrimSpace(cfg.KnowledgeSpace.RedisAddr)
	if knowledgeRedisAddr == "" {
		knowledgeRedisAddr = queueRedisAddr
	}
	knowledgeRedisPassword := strings.TrimSpace(cfg.KnowledgeSpace.RedisPassword)
	if knowledgeRedisPassword == "" {
		knowledgeRedisPassword = queueRedisPassword
	}
	knowledgeRedisDB := cfg.KnowledgeSpace.RedisDB
	if knowledgeRedisDB == 0 {
		knowledgeRedisDB = queueRedisDB
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
				BasePath:           cfg.Storage.Local.BasePath,
				PublicBaseURL:      cfg.Storage.Local.PublicBaseURL,
				UploadTokenSecret:  localTokenSecret,
				PublicTokenSecret:  publicTokenSecret,
				MaxUploadSizeBytes: maxUploadSize,
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
		Queue: shared.QueueOptions{
			Driver: cfg.Queue.Driver,
			Kafka: shared.QueueKafkaOptions{
				Brokers:       append([]string{}, cfg.Queue.Kafka.Brokers...),
				TopicPrefix:   cfg.Queue.Kafka.TopicPrefix,
				ConsumerGroup: cfg.Queue.Kafka.ConsumerGroup,
				PollTimeoutMs: cfg.Queue.Kafka.PollTimeoutMs,
			},
			Rabbit: shared.QueueRabbitMQOptions{
				URL:           cfg.Queue.Rabbit.URL,
				Exchange:      cfg.Queue.Rabbit.Exchange,
				QueuePrefix:   cfg.Queue.Rabbit.QueuePrefix,
				ConsumerTag:   cfg.Queue.Rabbit.ConsumerTag,
				Prefetch:      cfg.Queue.Rabbit.Prefetch,
				PollTimeoutMs: cfg.Queue.Rabbit.PollTimeoutMs,
			},
			NATS: shared.QueueNATSOptions{
				URLs:          append([]string{}, cfg.Queue.NATS.URLs...),
				SubjectPrefix: cfg.Queue.NATS.SubjectPrefix,
				QueueGroup:    cfg.Queue.NATS.QueueGroup,
				PollTimeoutMs: cfg.Queue.NATS.PollTimeoutMs,
			},
		},
		EventFabric: shared.EventFabricOptions{
			AckTimeoutSeconds: cfg.Event.Fabric.AckTimeoutSeconds,
			DefaultMaxRetry:   cfg.Event.Fabric.DefaultMaxRetry,
			RedisAddr:         eventRedisAddr,
			RedisPassword:     eventRedisPassword,
			RedisDB:           eventRedisDB,
			RetryKeyPrefix:    cfg.Event.Fabric.RetryKeyPrefix,
			ReplayKeyPrefix:   cfg.Event.Fabric.ReplayKeyPrefix,
			SchedulerInterval: schedulerInterval,
			Security: security.Config{
				RequireTLS:           cfg.Event.Fabric.Security.RequireTLS,
				SignatureSecret:      cfg.Event.Fabric.Security.SignatureSecret,
				SignatureHeader:      cfg.Event.Fabric.Security.SignatureHeader,
				TimestampHeader:      cfg.Event.Fabric.Security.TimestampHeader,
				SignatureKeyID:       cfg.Event.Fabric.Security.SignatureKeyID,
				AllowedClockSkew:     time.Duration(cfg.Event.Fabric.Security.AllowedClockSkewSeconds) * time.Second,
				ProtectedGRPCService: "/powerx.event_fabric.v1.",
				Sandbox: security.SandboxConfig{
					Enforce:              cfg.Event.Fabric.Security.Sandbox.Enforce,
					AllowedOutboundHosts: cfg.Event.Fabric.Security.Sandbox.AllowedOutboundHosts,
					BlockedHTTPPaths:     cfg.Event.Fabric.Security.Sandbox.BlockedHTTPPaths,
					BlockedGRPCMethods:   cfg.Event.Fabric.Security.Sandbox.BlockedGRPCMethods,
					ForbiddenHeaders:     cfg.Event.Fabric.Security.Sandbox.ForbiddenHeaders,
				},
			},
			Authorization: shared.EventFabricAuthorizationOptions{
				CacheTTLSeconds:             cfg.Event.Fabric.Authorization.CacheTTLSeconds,
				LocalCacheTTLSeconds:        cfg.Event.Fabric.Authorization.LocalCacheTTLSeconds,
				RedisAddr:                   authRedisAddr,
				RedisPassword:               authRedisPassword,
				RedisDB:                     authRedisDB,
				CacheInvalidateChannel:      cfg.Event.Fabric.Authorization.CacheInvalidateChannel,
				ChallengeSLASeconds:         cfg.Event.Fabric.Authorization.ChallengeSLASeconds,
				ChallengeTopic:              cfg.Event.Fabric.Authorization.ChallengeTopic,
				ChallengeConsumerGroup:      cfg.Event.Fabric.Authorization.ChallengeConsumerGroup,
				AlertTopic:                  cfg.Event.Fabric.Authorization.AlertTopic,
				RateLimitPrefix:             cfg.Event.Fabric.Authorization.RateLimitPrefix,
				TimeoutSweepIntervalSeconds: cfg.Event.Fabric.Authorization.TimeoutSweepIntervalSeconds,
				AuditRetentionDays:          cfg.Event.Fabric.Authorization.AuditRetentionDays,
				AuditArchiveBucket:          cfg.Event.Fabric.Authorization.AuditArchiveBucket,
				AuditArchivePrefix:          cfg.Event.Fabric.Authorization.AuditArchivePrefix,
				Secrets: shared.EventFabricAuthorizationSecretsOptions{
					Provider:                cfg.Event.Fabric.Authorization.Secrets.Provider,
					KeyID:                   cfg.Event.Fabric.Authorization.Secrets.KeyID,
					RotationIntervalSeconds: cfg.Event.Fabric.Authorization.Secrets.RotationIntervalSeconds,
					CacheTTLSeconds:         cfg.Event.Fabric.Authorization.Secrets.CacheTTLSeconds,
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
		CapabilityRegistry: shared.CapabilityRegistryOptions{
			Notifications: shared.CapabilityRegistryNotificationOptions{
				IMWebhook:        cfg.CapabilityRegistry.Notifications.IMWebhook,
				RetryInterval:    time.Duration(cfg.CapabilityRegistry.Notifications.RetryIntervalSec) * time.Second,
				RetryMaxAttempts: cfg.CapabilityRegistry.Notifications.RetryMaxAttempts,
				HTTPTimeout:      time.Duration(cfg.CapabilityRegistry.Notifications.HTTPTimeoutSec) * time.Second,
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
			StateBusTopics: shared.AgentLifecycleStateBusTopicsOptions{
				Lifecycle: cfg.AgentLifecycle.StateBusTopics.Lifecycle,
				Health:    cfg.AgentLifecycle.StateBusTopics.Health,
			},
			ShareReviewInterval: time.Duration(cfg.AgentLifecycle.ShareReviewDays) * 24 * time.Hour,
			Notifications: shared.AgentLifecycleNotificationOptions{
				IMWebhook:        cfg.AgentLifecycle.Notifications.IMWebhook,
				RetryInterval:    time.Duration(cfg.AgentLifecycle.Notifications.RetryIntervalSec) * time.Second,
				RetryMaxAttempts: cfg.AgentLifecycle.Notifications.RetryMaxAttempts,
				HTTPTimeout:      time.Duration(cfg.AgentLifecycle.Notifications.HTTPTimeoutSec) * time.Second,
			},
		},
		KnowledgeSpace: shared.KnowledgeSpaceOptions{
			RedisAddr:                knowledgeRedisAddr,
			RedisPassword:            knowledgeRedisPassword,
			RedisDB:                  knowledgeRedisDB,
			LockKeyPrefix:            cfg.KnowledgeSpace.LockKeyPrefix,
			MetricsKeyPrefix:         cfg.KnowledgeSpace.MetricsKeyPrefix,
			DefaultRetentionMonths:   cfg.KnowledgeSpace.DefaultRetentionMonths,
			ProvisioningSLA:          time.Duration(cfg.KnowledgeSpace.ProvisioningSLASeconds) * time.Second,
			IngestionSLA:             time.Duration(cfg.KnowledgeSpace.IngestionSLASeconds) * time.Second,
			SceneStrategyCatalogPath: cfg.KnowledgeSpace.SceneStrategyCatalogPath,
			IngestionProcessors: shared.KnowledgeSpaceIngestionProcessorOptions{
				PDFTextAvailable: cfg.KnowledgeSpace.IngestionProcessors.PDFTextAvailable,
				OCRAvailable:     cfg.KnowledgeSpace.IngestionProcessors.OCRAvailable,
			},
			EventTopics: shared.KnowledgeSpaceEventTopicsOptions{
				Provisioning: cfg.KnowledgeSpace.EventTopics.Provisioning,
				Ingestion:    cfg.KnowledgeSpace.EventTopics.Ingestion,
				Fusion:       cfg.KnowledgeSpace.EventTopics.Fusion,
				Feedback:     cfg.KnowledgeSpace.EventTopics.Feedback,
			},
			Notifications: shared.KnowledgeSpaceNotificationOptions{
				IMWebhook:        cfg.KnowledgeSpace.Notifications.IMWebhook,
				RetryInterval:    time.Duration(cfg.KnowledgeSpace.Notifications.RetryIntervalSec) * time.Second,
				RetryMaxAttempts: cfg.KnowledgeSpace.Notifications.RetryMaxAttempts,
				HTTPTimeout:      time.Duration(cfg.KnowledgeSpace.Notifications.HTTPTimeoutSec) * time.Second,
			},
			VectorStore: shared.KnowledgeSpaceVectorStoreOptions{
				Driver: cfg.KnowledgeSpace.VectorStore.Driver,
				PGVector: pgvectorcfg.Config{
					DSN: func() string {
						dsn := strings.TrimSpace(cfg.KnowledgeSpace.VectorStore.PgVector.DSN)
						if dsn != "" {
							return dsn
						}
						dsn = strings.TrimSpace(cfg.Database.DSN)
						if dsn != "" {
							return dsn
						}
						return composePostgresDSNFromDB(
							cfg.Database.Driver,
							cfg.Database.Host,
							cfg.Database.Port,
							cfg.Database.UserName,
							cfg.Database.Password,
							cfg.Database.Database,
							cfg.Database.SSLMode,
							cfg.Database.Timezone,
						)
					}(),
					Schema:           cfg.KnowledgeSpace.VectorStore.PgVector.Schema,
					Table:            cfg.KnowledgeSpace.VectorStore.PgVector.Table,
					Dimensions:       cfg.KnowledgeSpace.VectorStore.PgVector.Dimensions,
					EnableMigrations: cfg.KnowledgeSpace.VectorStore.PgVector.EnableMigrations,
					BatchSize:        cfg.KnowledgeSpace.VectorStore.PgVector.BatchSize,
					Lists:            cfg.KnowledgeSpace.VectorStore.PgVector.Lists,
					TimeoutSeconds:   cfg.KnowledgeSpace.VectorStore.PgVector.TimeoutSeconds,
				},
				Milvus: milvuscfg.Config{
					Endpoint: cfg.KnowledgeSpace.VectorStore.Milvus.Endpoint,
					APIKey:   cfg.KnowledgeSpace.VectorStore.Milvus.APIKey,
					Project:  cfg.KnowledgeSpace.VectorStore.Milvus.Project,
				},
				Pinecone: pineconecfg.Config{
					Endpoint:  cfg.KnowledgeSpace.VectorStore.Pinecone.Endpoint,
					APIKey:    cfg.KnowledgeSpace.VectorStore.Pinecone.APIKey,
					Index:     cfg.KnowledgeSpace.VectorStore.Pinecone.Index,
					Namespace: cfg.KnowledgeSpace.VectorStore.Pinecone.Namespace,
				},
			},
			Delta: shared.KnowledgeSpaceDeltaOptions{
				SourcesConfig:        cfg.KnowledgeSpace.Delta.SourcesConfig,
				PartialReleaseConfig: cfg.KnowledgeSpace.Delta.PartialReleaseConfig,
				ReportPath:           cfg.KnowledgeSpace.Delta.ReportPath,
				AggregateReportPath:  cfg.KnowledgeSpace.Delta.AggregateReportPath,
				SLAMinutes:           cfg.KnowledgeSpace.Delta.SLAMinutes,
				ApprovalMinutes:      cfg.KnowledgeSpace.Delta.ApprovalMinutes,
				DefaultDiffAccuracy:  cfg.KnowledgeSpace.Delta.DefaultDiffAccuracy,
			},
			Reports: shared.KnowledgeSpaceReportOptions{
				FeedbackPath: cfg.KnowledgeSpace.Reports.FeedbackPath,
				QABridgePath: cfg.KnowledgeSpace.Reports.QABridgePath,
			},
			EventHotfix: shared.KnowledgeSpaceEventHotfixOptions{
				PoliciesPath:        cfg.KnowledgeSpace.EventHotfix.PoliciesPath,
				AgentMatrixPath:     cfg.KnowledgeSpace.EventHotfix.AgentMatrixPath,
				ReportPath:          cfg.KnowledgeSpace.EventHotfix.ReportPath,
				AggregateReportPath: cfg.KnowledgeSpace.EventHotfix.AggregateReportPath,
				RetryMax:            cfg.KnowledgeSpace.EventHotfix.RetryMax,
				ReplayWindow:        time.Duration(cfg.KnowledgeSpace.EventHotfix.ReplayWindowSeconds) * time.Second,
			},
			Decay: shared.KnowledgeSpaceDecayOptions{
				ThresholdPath:       cfg.KnowledgeSpace.Decay.ThresholdPath,
				ReportPath:          cfg.KnowledgeSpace.Decay.ReportPath,
				AggregateReportPath: cfg.KnowledgeSpace.Decay.AggregateReportPath,
			},
			Release: shared.KnowledgeSpaceReleaseOptions{
				MatrixPath:          cfg.KnowledgeSpace.Release.MatrixPath,
				GuardrailsDoc:       cfg.KnowledgeSpace.Release.GuardrailsDoc,
				ReportPath:          cfg.KnowledgeSpace.Release.ReportPath,
				AggregateReportPath: cfg.KnowledgeSpace.Release.AggregateReportPath,
			},
		},
		PluginRelease: shared.PluginReleaseOptions{
			FeatureFlags: shared.PluginReleaseFeatureFlagsOptions{
				EnableLocalInstall:        cfg.Plugin.Release.FeatureFlags.EnableLocalInstall,
				EnablePipelineDeployment:  cfg.Plugin.Release.FeatureFlags.EnablePipelineDeployment,
				EnableOfflineDistribution: cfg.Plugin.Release.FeatureFlags.EnableOfflineDistribution,
			},
			LocalInstall: shared.PluginReleaseLocalInstallOptions{
				SessionTTL:        time.Duration(cfg.Plugin.Release.LocalInstall.SessionTTLMinutes) * time.Minute,
				MaxArtifactSizeMB: cfg.Plugin.Release.LocalInstall.MaxArtifactSizeMB,
			},
			Pipeline: shared.PluginReleasePipelineOptions{
				ApprovalSLA:           time.Duration(cfg.Plugin.Release.Pipeline.ApprovalSLAHours) * time.Hour,
				MaxParallelReleases:   cfg.Plugin.Release.Pipeline.MaxParallelReleases,
				DefaultRollbackNotice: time.Duration(cfg.Plugin.Release.Pipeline.DefaultRollbackNotice) * time.Minute,
			},
			Canary: shared.PluginReleaseCanaryOptions{
				RollbackTimeout:  time.Duration(cfg.Plugin.Release.Canary.RollbackTimeoutSeconds) * time.Second,
				DefaultBatchSize: cfg.Plugin.Release.Canary.DefaultBatchSize,
				MaxBatches:       cfg.Plugin.Release.Canary.MaxBatches,
			},
			Distribution: shared.PluginReleaseDistributionOptions{
				OfflineBucket:       cfg.Plugin.Release.Distribution.OfflineBucket,
				OfflinePrefix:       cfg.Plugin.Release.Distribution.OfflinePrefix,
				EscalationThreshold: cfg.Plugin.Release.Distribution.EscalationThreshold,
				ArtifactRetention:   time.Duration(cfg.Plugin.Release.Distribution.ArtifactRetentionDays) * 24 * time.Hour,
			},
			Observability: shared.PluginReleaseObservabilityOptions{
				DashboardUID:    cfg.Plugin.Release.Observability.DashboardUID,
				AlertRulePrefix: cfg.Plugin.Release.Observability.AlertRulePrefix,
				KPITargets: shared.PluginReleaseKPITargetsOptions{
					CanRollbackWithin: time.Duration(cfg.Plugin.Release.Observability.KPITargets.CanRollbackWithinSeconds) * time.Second,
					HotloadLatencyP95: time.Duration(cfg.Plugin.Release.Observability.KPITargets.HotloadLatencyP95Ms) * time.Millisecond,
				},
			},
		},
		PluginBootstrap: shared.PluginBootstrapOptions{
			TemplatesPath:   cfg.Plugin.Bootstrap.TemplatesIndex,
			DefaultTemplate: cfg.Plugin.Bootstrap.DefaultTemplate,
			AllowHosts:      cfg.Plugin.Bootstrap.AllowlistedHosts,
		},
		PluginDebug: shared.PluginDebugOptions{
			Component: strings.TrimSpace(cfg.Plugin.Debug.Component),
			HostSimulator: shared.PluginDebugHostOptions{
				Enabled:     cfg.Plugin.Debug.HostSimulator.Enabled,
				FeatureFlag: cfg.Plugin.Debug.HostSimulator.FeatureFlag,
				ConfigPath:  cfg.Plugin.Debug.HostSimulator.ConfigPath,
			},
			Reports: shared.PluginDebugReportOptions{
				TemplatePath:     cfg.Plugin.Debug.Reports.TemplatePath,
				MaskingRulesPath: cfg.Plugin.Debug.Reports.MaskingRules,
				FallbackLogBase:  cfg.Plugin.Debug.Reports.FallbackLogBase,
			},
			TicketBridge: shared.PluginDebugTicketBridgeOptions{
				Provider: cfg.Plugin.Debug.TicketBridge.Provider,
				Endpoint: cfg.Plugin.Debug.TicketBridge.Endpoint,
				Project:  cfg.Plugin.Debug.TicketBridge.Project,
			},
			Sandbox: shared.PluginDebugSandboxOptions{
				Enabled:       cfg.Plugin.Debug.Sandbox.Enabled,
				FeatureFlag:   cfg.Plugin.Debug.Sandbox.FeatureFlag,
				DataSuitePath: cfg.Plugin.Debug.Sandbox.DataSuitePath,
			},
		},
		Server: shared.ServerOptions{
			GRPC: shared.GRPCServerOptions{
				Host: cfg.Server.GRPC.Host,
				Port: cfg.Server.GRPC.Port,
			},
		},
		DevHotload: shared.DevHotloadOptions{
			FeatureFlags: shared.DevHotloadFeatureFlagsOptions{
				Enabled:          cfg.Plugin.DevHotload.FeatureFlags.Enabled,
				GatewayFlag:      cfg.Plugin.DevHotload.FeatureFlags.GatewayFlag,
				SessionAuditFlag: cfg.Plugin.DevHotload.FeatureFlags.SessionAuditFlag,
			},
			Sessions: shared.DevHotloadSessionOptions{
				TTL:             time.Duration(cfg.Plugin.DevHotload.Sessions.TTLMinutes) * time.Minute,
				MaxConcurrent:   cfg.Plugin.DevHotload.Sessions.MaxConcurrentSessions,
				CleanupInterval: time.Duration(cfg.Plugin.DevHotload.Sessions.CleanupIntervalSeconds) * time.Second,
			},
			Sandbox: shared.DevHotloadSandboxOptions{
				Image:          cfg.Plugin.DevHotload.Sandbox.Image,
				MaxCPUPercent:  cfg.Plugin.DevHotload.Sandbox.MaxCPUPercent,
				MaxMemoryMB:    cfg.Plugin.DevHotload.Sandbox.MaxMemoryMB,
				WatchFileLimit: cfg.Plugin.DevHotload.Sandbox.WatchFileLimit,
			},
			Security: shared.DevHotloadSecurityOptions{
				RequireMTLS:     cfg.Plugin.DevHotload.Security.RequireMTLS,
				AllowedSubjects: cfg.Plugin.DevHotload.Security.AllowedSubjects,
				PATHeader:       cfg.Plugin.DevHotload.Security.PATHeader,
				TokenTTL:        time.Duration(cfg.Plugin.DevHotload.Security.TokenTTLSeconds) * time.Second,
				TokenSecret:     []byte(cfg.Auth.JWTSecret),
				TokenIssuer:     cfg.Auth.Issuer,
				TokenAudience:   cfg.Auth.AudienceUser,
				TokenPlatforms:  cfg.Auth.Platforms,
				TokenRoles:      []string{"system_admin"},
				ImpersonateRoot: true,
			},
			Observability: shared.DevHotloadObservabilityOptions{
				MetricsNamespace: cfg.Plugin.DevHotload.Observability.MetricsNamespace,
				SSEBufferSize:    cfg.Plugin.DevHotload.Observability.SSEBufferSize,
				AuditTopic:       cfg.Plugin.DevHotload.Observability.AuditTopic,
			},
		},
	}

	deps := shared.NewDeps(db, opts)

	return deps, nil
}

func buildEventBusConfig(cfg *config.Config, defaultAddr, defaultPassword string, defaultDB int) *event_bus.Config {
	busCfg := &event_bus.Config{Type: strings.TrimSpace(cfg.Event.Bus.Type)}
	if busCfg.Type == "" {
		busCfg.Type = "local"
	}
	if strings.EqualFold(busCfg.Type, "redis") {
		addr := strings.TrimSpace(cfg.Event.Bus.RedisAddr)
		if addr == "" {
			addr = defaultAddr
		}
		password := strings.TrimSpace(cfg.Event.Bus.RedisPassword)
		if password == "" {
			password = defaultPassword
		}
		db := cfg.Event.Bus.RedisDB
		if db == 0 {
			db = defaultDB
		}
		busCfg.Redis = &event_bus.RedisConfig{
			Addr:     addr,
			Password: password,
			DB:       db,
		}
	}
	return busCfg
}
