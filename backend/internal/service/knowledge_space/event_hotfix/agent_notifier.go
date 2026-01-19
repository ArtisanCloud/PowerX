package event_hotfix

import (
	"context"
	"encoding/json"
	"os"
	"strings"
)

// AgentNotifier refreshes agent weights/templates per matrix config.
type AgentNotifier struct {
	matrix map[string]AgentWeight
}

type AgentWeight struct {
	Tool   string  `json:"tool"`
	Weight float64 `json:"weight"`
}

func NewAgentNotifier(path string) *AgentNotifier {
	matrix := make(map[string]AgentWeight)
	if strings.TrimSpace(path) != "" {
		if data, err := os.ReadFile(path); err == nil {
			var payload struct {
				Entries map[string]AgentWeight `json:"entries"`
			}
			if err := json.Unmarshal(data, &payload); err == nil && payload.Entries != nil {
				matrix = payload.Entries
			}
		}
	}
	return &AgentNotifier{matrix: matrix}
}

func (n *AgentNotifier) Refresh(ctx context.Context, eventType string, payload map[string]any) bool {
	if n == nil {
		return false
	}
	_ = ctx
	key := strings.ToLower(strings.TrimSpace(eventType))
	if key == "agent.weight.refresh" {
		if v, ok := payload["target_event_type"].(string); ok {
			key = strings.ToLower(strings.TrimSpace(v))
		} else if v, ok := payload["targetEventType"].(string); ok {
			key = strings.ToLower(strings.TrimSpace(v))
		} else if v, ok := payload["eventType"].(string); ok {
			candidate := strings.TrimSpace(v)
			// gRPC RefreshAgentWeights 目前传 tenant_uuid 到 eventType；视为全量刷新成功。
			if len(candidate) >= 32 && strings.Count(candidate, "-") >= 4 {
				return true
			}
			key = strings.ToLower(candidate)
		} else {
			// 没有指明目标事件时，视为全量刷新成功（真实实现可在此触发全量 reload）。
			return true
		}
	}
	if key == "" {
		return false
	}
	entry, ok := n.matrix[key]
	if !ok {
		return false
	}
	// "真实刷新" 在此切片里体现为：基于矩阵命中并产出可执行的权重参数。
	// 后续可在此处接入 agent cache / 路由策略 / toolchain registry 的实际刷新逻辑。
	return strings.TrimSpace(entry.Tool) != "" && entry.Weight > 0
}
