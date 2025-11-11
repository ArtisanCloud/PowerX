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
		Plugin: DefaultPluginConfig(),
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
		EventBus: EventBusConfig{
			Type:          "local",
			RedisAddr:     "localhost:6379",
			RedisPassword: "",
			DedupeTTLSec:  30,
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
			Notifications: AgentLifecycleNotificationConfig{
				IMWebhook:        "",
				RetryIntervalSec: 30,
				RetryMaxAttempts: 3,
				HTTPTimeoutSec:   5,
			},
		},
		EventFabric: EventFabricConfig{
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
				CacheTTLSeconds:             600,
				LocalCacheTTLSeconds:        10,
				RedisAddr:                   "localhost:6379",
				RedisPassword:               "",
				RedisDB:                     1,
				CacheInvalidateChannel:      "event_fabric:authorization:invalidate",
				ChallengeSLASeconds:         900,
				ChallengeTopic:              "secops.challenge",
				ChallengeConsumerGroup:      "corex-authorization",
				AlertTopic:                  "secops.alerts",
				RateLimitPrefix:             "event_fabric:authorization:rl",
				TimeoutSweepIntervalSeconds: 30,
				AuditRetentionDays:          1095,
				AuditArchiveBucket:          "powerx-audit",
				AuditArchivePrefix:          "event-fabric/authorization",
				Secrets: EventFabricAuthorizationSecretsConfig{
					Provider:                "",
					KeyID:                   "",
					RotationIntervalSeconds: 0,
					CacheTTLSeconds:         900,
				},
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
		},
		FeatureGate: FeatureGateConfig{
			LicenseKey: "demo-license-xyz",
		},
		PluginRelease:   DefaultPluginReleaseConfig(),
		PluginBootstrap: DefaultPluginBootstrapConfig(),
		PluginDebug:     DefaultPluginDebugConfig(),
		Storage: StorageConfig{
			DefaultDriver: "local",
			TTLSeconds:    43200,
			Local: LocalStorageConfig{
				BasePath:             "./storage/media",
				PublicBaseURL:        "http://localhost:8077/media",
				EnableUploadEndpoint: false,
				UploadTokenSecret:    "",
				MaxUploadSizeBytes:   100 << 20, // 100MB
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
