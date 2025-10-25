package agentlifecycle_unit

import (
	"context"
	"testing"
	"time"

	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/stretchr/testify/require"
)

func TestRecordHealthSnapshotPublishesEventAndThrottlesAlerts(t *testing.T) {
	res := newTestResources(t)
	ctx := context.Background()
	profile := res.seedProfile("active")

	eventCh := make(chan map[string]any, 1)
	unsub := res.bus.Subscribe("agent.health.degraded", func(evt event_bus.Event) error {
		if wrap, ok := evt.Payload.(map[string]any); ok {
			if payload, ok := wrap["payload"].(map[string]any); ok {
				eventCh <- payload
			}
		}
		return nil
	})
	defer unsub()

	input := agent_lifecycle.HealthInput{
		AgentID:        profile.UUID,
		TenantID:       profile.TenantID,
		WindowDuration: time.Minute,
		Status:         "degraded",
		Metrics: agent_lifecycle.HealthMetricsInput{
			ThroughputPerMin: 70,
			SuccessRate:      0.72,
			P95LatencyMs:     1800,
			ResourceUtilPct:  0.88,
			ErrorRate:        0.32,
		},
	}

	require.NoError(t, res.service.RecordHealthSnapshot(ctx, input))

	select {
	case payload := <-eventCh:
		require.Equal(t, "degraded", payload["status"])
	case <-time.After(time.Second):
		t.Fatal("expected health degraded event")
	}
	require.Equal(t, 1, res.notifier.Count())

	res.advanceClock(30 * time.Second)
	require.NoError(t, res.service.RecordHealthSnapshot(ctx, input))
	require.Equal(t, 1, res.notifier.Count(), "cooldown should suppress duplicate alert")

	res.advanceClock(2 * time.Minute)
	require.NoError(t, res.service.RecordHealthSnapshot(ctx, input))
	require.Equal(t, 2, res.notifier.Count(), "alert should resume after cooldown")
}

func TestHealthSnapshotRepositoryUpsert(t *testing.T) {
	res := newTestResources(t)
	ctx := context.Background()
	profile := res.seedProfile("active")
	window := res.currentTS.Truncate(time.Minute)

	record := &agentmodel.AgentHealthSnapshotRecord{
		AgentUUID:         profile.UUID,
		TenantID:          profile.TenantID,
		WindowStartedAt:   window,
		WindowDurationSec: 60,
		HealthScore:       90,
		Status:            "healthy",
	}
	_, err := res.health.Upsert(ctx, record)
	require.NoError(t, err)

	record.HealthScore = 45
	record.Status = "degraded"
	_, err = res.health.Upsert(ctx, record)
	require.NoError(t, err)

	var stored agentmodel.AgentHealthSnapshotRecord
	require.NoError(t, res.db.WithContext(ctx).
		Where("agent_uuid = ?", profile.UUID).
		First(&stored).Error)
	require.Equal(t, int32(45), stored.HealthScore)
	require.Equal(t, "degraded", stored.Status)

	var count int64
	require.NoError(t, res.db.WithContext(ctx).
		Model(&agentmodel.AgentHealthSnapshotRecord{}).
		Where("agent_uuid = ?", profile.UUID).
		Count(&count).Error)
	require.Equal(t, int64(1), count, "upsert should not create duplicate rows")
}
