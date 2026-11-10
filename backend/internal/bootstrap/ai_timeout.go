package bootstrap

import (
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"time"
)

// capabilityRegistryOptions 将同一份 AI 配置传入代理，不另设推理或排队期限。
func capabilityRegistryOptions(cfg *config.Config) shared.CapabilityRegistryOptions {
	return shared.CapabilityRegistryOptions{
		DefaultHTTPTimeout:      time.Duration(cfg.CapabilityRegistry.DefaultHTTPTimeoutSeconds) * time.Second,
		AIMultimodalHTTPTimeout: time.Duration(cfg.CapabilityRegistry.AIMultimodalHTTPTimeoutSeconds) * time.Second,
		LLMRequestTimeout:       cfg.AI.Defaults.LLM.RequestTimeout,
		LLMQueueWaitTimeout:     cfg.AI.Runtime.QueueWaitTimeout,
		Notifications: shared.CapabilityRegistryNotificationOptions{
			IMWebhook:        cfg.CapabilityRegistry.Notifications.IMWebhook,
			RetryInterval:    time.Duration(cfg.CapabilityRegistry.Notifications.RetryIntervalSec) * time.Second,
			RetryMaxAttempts: cfg.CapabilityRegistry.Notifications.RetryMaxAttempts,
			HTTPTimeout:      time.Duration(cfg.CapabilityRegistry.Notifications.HTTPTimeoutSec) * time.Second,
		},
	}
}
