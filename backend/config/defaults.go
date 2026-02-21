package config

import (
	agentCfg "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	dbCfg "github.com/ArtisanCloud/PowerX/pkg/corex/db"
	logCfg "github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
)

// GetDefaults 返回默认配置（已对齐新版 AuthConfig 字段）
func GetDefaults() *Config {
	return &Config{
		Server: ServerConfig{
			Port:                8077,
			ReadTimeoutSeconds:  5,
			WriteTimeoutSeconds: 10,
			Mode:                "debug",
			APIPrefix:           "/api", // 如需
		},
		HTTPSecurity: HTTPSecurityConfig{
			FrameAncestors: []string{
				"'self'",
				"http://localhost:3030",
				"http://127.0.0.1:3030",
				"https://admin.powerx.io",
			},
		},
		Tenants: TenantConfig{
			RequireUUID: true,
		},
		Plugin: DefaultPluginAggregateConfig(),
		LogConfig: logCfg.LogConfig{
			Level:         "debug",
			Console:       true,
			UseJsonFormat: false,
			File: logCfg.FileConfig{
				Enable:        false,
				InfoFilePath:  "logs/info.log",
				ErrorFilePath: "logs/error.log",
				MaxSize:       100,
				MaxBackups:    5,
				MaxAge:        30,
				Compress:      true,
			},
			Loki: logCfg.LokiConfig{
				Enable:    false,
				URL:       "",
				JobName:   "corex",
				BatchWait: 1,
				BatchSize: 100,
			},
			HttpDebug: false,
			Debug:     true,
		},
		Auth: AuthConfig{
			JWTSecret:        "K8mN2pQ7rS9tU4vW6xY1zA3bC5dE8fG0",
			Issuer:           "powerx-auth",
			AudienceUser:     "user",
			AudienceCustomer: "customer",
			Platforms:        []string{"admin", "web", "miniapp"},
			AccessTTLStr:     "15m",
			RefreshTTLStr:    "336h", // 14d
		},
		Event: EventConfig{
			Bus: EventBusConfig{
				Type:          "local",
				RedisAddr:     "",
				RedisPassword: "",
				RedisDB:       0,
				DedupeTTLSec:  30,
			},
			Fabric: EventFabricConfig{
				AckTimeoutSeconds: 30,
				DefaultMaxRetry:   5,
				RedisAddr:         "localhost:6379",
				RedisPassword:     "",
				RedisDB:           0,
				RetryKeyPrefix:    "event_fabric:retry",
				ReplayKeyPrefix:   "event_fabric:replay",
				SchedulerInterval: 5,
				Security: EventFabricSecurityConfig{
					RequireTLS:              false,
					SignatureHeader:         "X-PowerX-Signature",
					TimestampHeader:         "X-PowerX-Timestamp",
					SignatureKeyID:          "event-fabric",
					AllowedClockSkewSeconds: 300,
					Sandbox: EventFabricSecuritySandboxConfig{
						Enforce:              false,
						AllowedOutboundHosts: []string{},
						BlockedHTTPPaths:     []string{},
						BlockedGRPCMethods:   []string{},
						ForbiddenHeaders:     []string{},
					},
				},
				Authorization: EventFabricAuthorizationConfig{
					CacheTTLSeconds:             60,
					LocalCacheTTLSeconds:        30,
					RedisAddr:                   "localhost:6379",
					RedisPassword:               "",
					RedisDB:                     1,
					CacheInvalidateChannel:      "event_fabric:authorization:invalidate",
					ChallengeSLASeconds:         60,
					ChallengeTopic:              "event_fabric.authorization.challenge",
					ChallengeConsumerGroup:      "event_fabric.authorization.default",
					AlertTopic:                  "event_fabric.authorization.alert",
					RateLimitPrefix:             "event_fabric:authorization:rl",
					TimeoutSweepIntervalSeconds: 60,
					AuditRetentionDays:          7,
					Secrets: EventFabricAuthorizationSecretsConfig{
						Provider:                "",
						KeyID:                   "",
						RotationIntervalSeconds: 0,
						CacheTTLSeconds:         0,
					},
				},
			},
		},
		Queue: QueueConfig{
			Driver: "redis",
			Redis: QueueRedisConfig{
				Addr:     "localhost:6379",
				Password: "",
				DB:       5,
			},
			Kafka: QueueKafkaConfig{
				Brokers:       []string{"localhost:9092"},
				TopicPrefix:   "event_fabric.task",
				ConsumerGroup: "powerx.event_fabric",
				PollTimeoutMs: 1000,
			},
			Rabbit: QueueRabbitMQConfig{
				URL:           "amqp://guest:guest@localhost:5672/",
				Exchange:      "event_fabric.task",
				QueuePrefix:   "event_fabric.task",
				ConsumerTag:   "powerx.event_fabric",
				Prefetch:      50,
				PollTimeoutMs: 1000,
			},
			NATS: QueueNATSConfig{
				URLs:          []string{"nats://localhost:4222"},
				SubjectPrefix: "event_fabric.task",
				QueueGroup:    "powerx.event_fabric",
				PollTimeoutMs: 1000,
			},
		},
		Scheduler: SchedulerConfig{
			Driver:          "builtin",
			IntervalSeconds: 5,
		},
		IntegrationGateway: IntegrationGatewayConfig{
			RateLimitPrefix: "integration_gateway:rl",
			DefaultRateLimit: IntegrationGatewayRateLimitConfig{
				Limit:         120,
				Burst:         120,
				WindowSeconds: 60,
				Scope:         "per_route_per_tenant",
			},
			EventTopics: IntegrationGatewayEventTopics{
				Created:             "integration.gateway.route.created",
				Updated:             "integration.gateway.route.updated",
				InvocationSucceeded: "integration.gateway.invocation.succeeded",
				InvocationFailed:    "integration.gateway.invocation.failed",
			},
			RedisAddr:     "localhost:6379",
			RedisPassword: "",
			RedisDB:       2,
		},
		CapabilityRegistry: CapabilityRegistryConfig{
			RedisPrefix:      "capability_registry:cache",
			EventTopicPrefix: "capability.catalog",
			DefaultRateLimit: CapabilityRegistryRateLimitConfig{
				Limit:         60,
				Burst:         120,
				WindowSeconds: 60,
			},
			Notifications: CapabilityRegistryNotificationConfig{
				IMWebhook:        "",
				RetryIntervalSec: 30,
				RetryMaxAttempts: 3,
				HTTPTimeoutSec:   5,
			},
		},
		AgentLifecycle: AgentLifecycleConfig{
			RedisAddr:                "localhost:6379",
			RedisPassword:            "",
			RedisDB:                  3,
			CapacityKeyPrefix:        "agent_lifecycle:capacity",
			HealthKeyPrefix:          "agent_lifecycle:health",
			DefaultCapacityInstances: 3,
			EventTopics: AgentLifecycleEventTopics{
				LifecyclePrefix: "agent.lifecycle",
				HealthPrefix:    "agent.health",
			},
			StateBusTopics: AgentLifecycleStateBusTopics{
				Lifecycle: "statebus.agent.lifecycle",
				Health:    "statebus.agent.health",
			},
			ShareReviewDays: 30,
			Notifications: AgentLifecycleNotificationConfig{
				IMWebhook:        "",
				RetryIntervalSec: 30,
				RetryMaxAttempts: 3,
				HTTPTimeoutSec:   5,
			},
		},
		KnowledgeSpace: KnowledgeSpaceConfig{
			RedisAddr:                "",
			RedisPassword:            "",
			RedisDB:                  0,
			LockKeyPrefix:            "knowledge_space:lock",
			MetricsKeyPrefix:         "knowledge_space:metrics",
			DefaultRetentionMonths:   13,
			ProvisioningSLASeconds:   120,
			IngestionSLASeconds:      4 * 3600,
			SceneStrategyCatalogPath: "backend/config/knowledge/scene_strategy_catalog.yaml",
			EventTopics: KnowledgeSpaceEventTopics{
				Provisioning: "knowledge.space.provisioning",
				Ingestion:    "knowledge.space.ingestion",
				Fusion:       "knowledge.space.fusion",
				Feedback:     "knowledge.space.feedback",
			},
			Notifications: KnowledgeSpaceNotificationConfig{
				IMWebhook:        "",
				RetryIntervalSec: 60,
				RetryMaxAttempts: 3,
				HTTPTimeoutSec:   5,
			},
			VectorStore: KnowledgeSpaceVectorStoreConfig{
				Driver: "",
				PgVector: KnowledgeSpaceVectorStorePGVectorConfig{
					Schema:           "public",
					Table:            "knowledge_vectors_v1_1536",
					Dimensions:       1536,
					EnableMigrations: false,
					BatchSize:        128,
					Lists:            100,
					TimeoutSeconds:   30,
				},
			},
			// B 方案默认：使用 Postgres-backed 的 sparse/hier/structured/kg（外部实现可覆盖为 external）。
			IndexBackends: KnowledgeSpaceIndexBackendConfig{
				Sparse:           "postgres_fts",
				Hier:             "postgres_links",
				StructuredFields: "postgres_jsonb",
				KG:               "postgres",
			},
			Delta: KnowledgeSpaceDeltaConfig{
				SourcesConfig:        "backend/config/knowledge/delta_sources.yaml",
				PartialReleaseConfig: "backend/config/knowledge/partial_release.yaml",
				ReportPath:           "backend/reports/_state/knowledge-delta.json",
				AggregateReportPath:  "reports/_state/knowledge-update.json",
				SLAMinutes:           30,
				ApprovalMinutes:      15,
				DefaultDiffAccuracy:  98.0,
			},
			Reports: KnowledgeSpaceReportConfig{
				FeedbackPath: "backend/reports/_state/knowledge-feedback.json",
				QABridgePath: "reports/_state/qa-reasoning.json",
			},
			EventHotfix: KnowledgeSpaceEventHotfixConfig{
				PoliciesPath:        "backend/config/knowledge/event_hotfix_policies.yaml",
				AgentMatrixPath:     "backend/config/knowledge/agent_weight_matrix.yaml",
				ReportPath:          "backend/reports/_state/knowledge-event.json",
				AggregateReportPath: "reports/_state/knowledge-update.json",
				RetryMax:            3,
				ReplayWindowSeconds: 300,
			},
			Decay: KnowledgeSpaceDecayConfig{
				ThresholdPath:       "backend/config/knowledge/decay_thresholds.yaml",
				ReportPath:          "backend/reports/_state/knowledge-decay.json",
				AggregateReportPath: "reports/_state/knowledge-update.json",
			},
			Release: KnowledgeSpaceReleaseConfig{
				MatrixPath:          "backend/config/knowledge/tenant_release_matrix.yaml",
				GuardrailsDoc:       "docs/ops/release_guardrails.md",
				ReportPath:          "backend/reports/_state/knowledge-release.json",
				AggregateReportPath: "reports/_state/knowledge-update.json",
			},
		},
		LowCode: LowCodeConfig{
			MaxConcurrentFlows: 10,
			DefaultTimeoutSec:  60,
		},
		Agent: agentCfg.AgentConfig{
			Host: "127.0.0.1",
			Port: 8082,
			Mode: "ws_sse",
			FlowSpec: agentCfg.FlowSpecConfig{
				BaseDir:     "./pkg/corex/flow/blueprints",
				BusinessDir: "./internal/server/agent/blueprints",
			},
			TemplateDir: "./services/agent/templates",
		},
		Database: dbCfg.DatabaseConfig{
			Host:                   "localhost",
			Port:                   5432,
			UserName:               "postgres",
			Password:               "postgres",
			Database:               "corex",
			SSLMode:                "disable",
			Timezone:               "Asia/Shanghai",
			TablePrefix:            "px_",
			MaxIdleConns:           10,
			MaxOpenConns:           100,
			ConnMaxLifetimeMinutes: 60,
			LogLevel:               "info",
			ClusterMode:            "single",
			Replicas:               nil,
		},
		FeatureGate: FeatureGateConfig{
			LicenseKey:                 "demo-license-xyz",
			EnableEventFabric:          false,
			EnableWorkflow:             true,
			EnableKnowledgeSpace:       true,
			EnableMediaPlatform:        true,
			EnableExperimentalFeatures: true,
		},
		Storage: StorageConfig{
			DefaultDriver: "local",
			TTLSeconds:    43200,
			Local: LocalStorageConfig{
				BasePath:           "./storage/media",
				PublicBaseURL:      "http://localhost:8077/media",
				UploadTokenSecret:  "",
				PublicTokenSecret:  "",
				MaxUploadSizeBytes: 100 << 20, // 100MB
			},
			S3: S3StorageConfig{
				Endpoint:       "http://127.0.0.1:9000",
				Region:         "us-east-1",
				AccessKey:      "minioadmin",
				SecretKey:      "minioadmin",
				Bucket:         "powerx-media",
				UseSSL:         false,
				ForcePathStyle: true,
			},
		},
	}
}
