package agent_lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"github.com/google/uuid"
)

// AuditBridge 聚合 Agent 状态、健康与事件，供 ReAct/任务执行桥接使用。
type AuditBridge struct {
	profiles *agentrepo.AgentProfileLifecycleRepository
	events   *agentrepo.AgentLifecycleEventRepository
	health   *agentrepo.AgentHealthSnapshotRepository
}

// NewAuditBridge 构造审计桥接器。
func NewAuditBridge(
	profiles *agentrepo.AgentProfileLifecycleRepository,
	events *agentrepo.AgentLifecycleEventRepository,
	health *agentrepo.AgentHealthSnapshotRepository,
) *AuditBridge {
	if profiles == nil || events == nil {
		return nil
	}
	return &AuditBridge{
		profiles: profiles,
		events:   events,
		health:   health,
	}
}

// AgentBridgeState 汇总 Agent 状态、健康与事件。
type AgentBridgeState struct {
	Agent  *Agent                 `json:"agent"`
	Health *HealthSummary         `json:"health"`
	Events []BridgeLifecycleEvent `json:"events"`
}

// BridgeLifecycleEvent 对外暴露的事件视图。
type BridgeLifecycleEvent struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	From        string `json:"from_status"`
	To          string `json:"to_status"`
	Reason      string `json:"reason"`
	TriggeredBy string `json:"triggered_by"`
	TraceID     string `json:"trace_id"`
	OccurredAt  string `json:"occurred_at"`
}

// Snapshot 返回 Agent 的聚合状态。
func (b *AuditBridge) Snapshot(ctx context.Context, agentID uuid.UUID, eventLimit int) (*AgentBridgeState, error) {
	if b == nil {
		return nil, errors.New("audit bridge not configured")
	}
	profile, err := b.profiles.GetByUUID(ctx, agentID)
	if err != nil {
		if errors.Is(err, agentrepo.ErrAgentProfileNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	agent := toAgent(profile, parseToolGrants(profile.ToolGrants), parseMetadata(profile.Metadata))

	var healthSummary *HealthSummary
	if b.health != nil {
		if snaps, err := b.health.ListByAgent(ctx, agentID, 0, 1); err == nil && len(snaps) > 0 {
			healthSummary = convertSnapshot(&snaps[0])
		}
	}

	if eventLimit <= 0 {
		eventLimit = 20
	}
	eventRecords, err := b.events.ListByAgent(ctx, agentID, eventLimit)
	if err != nil {
		return nil, fmt.Errorf("list lifecycle events: %w", err)
	}
	events := make([]BridgeLifecycleEvent, 0, len(eventRecords))
	for _, rec := range eventRecords {
		events = append(events, toBridgeEvent(&rec))
	}

	return &AgentBridgeState{
		Agent:  agent,
		Health: healthSummary,
		Events: events,
	}, nil
}

func toBridgeEvent(rec *agentmodel.AgentLifecycleEventRecord) BridgeLifecycleEvent {
	if rec == nil {
		return BridgeLifecycleEvent{}
	}
	return BridgeLifecycleEvent{
		ID:          rec.UUID.String(),
		Type:        rec.EventType,
		From:        rec.FromStatus,
		To:          rec.ToStatus,
		Reason:      rec.Reason,
		TriggeredBy: rec.TriggeredBy,
		TraceID:     rec.TraceID,
		OccurredAt:  rec.OccurredAt.UTC().Format(time.RFC3339),
	}
}
