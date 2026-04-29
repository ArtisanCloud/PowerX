package event_bus

import (
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// InitEventBus 订阅全局事件（示例 plugin 行为）
func InitEventBus(cfg *Config) error {
	// 初始化默认事件总线
	if cfg == nil {
		cfg = &Config{Type: "local"}
	}
	if cfg.Type == "" {
		cfg.Type = "local"
	}
	err := InitDefaultEventBus(cfg)
	if err != nil {
		return err
	}
	// 订阅认证成功事件
	Subscribe("auth_succeeded", func(e Event) error {
		if payload, ok := e.Payload.(map[string]interface{}); ok {
			logger.InfoF(e.Ctx, "[plugin] auth_succeeded: tenant_uuid=%v subject=%v platform=%v trace_id=%v scope=%v",
				payload["tenant_uuid"], payload["subject"], payload["platform"], payload["trace_id"], payload["scope"])
		}
		return nil
	})

	// 订阅流程完成事件
	Subscribe("flow_completed", func(e Event) error {
		if payload, ok := e.Payload.(map[string]interface{}); ok {
			logger.InfoF(e.Ctx, "[plugin] flow_completed: tenant_uuid=%v flow=%v subject=%v trace_id=%v",
				payload["tenant_uuid"], payload["flow_name"], payload["subject"], payload["trace_id"])
		}
		return nil
	})

	return err
}
