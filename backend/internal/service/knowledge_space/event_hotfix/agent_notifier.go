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

func (n *AgentNotifier) Refresh(_ context.Context, payload map[string]any) bool {
	if n == nil {
		return false
	}
	eventType, _ := payload["eventType"].(string)
	entry, ok := n.matrix[strings.ToLower(eventType)]
	return ok && entry.Weight > 0
}
