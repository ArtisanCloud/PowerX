package knowledge_space_integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	event_hotfix "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/event_hotfix"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestEventHotfixFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	svc := env.Deps.KnowledgeSpace.EventHotfix
	require.NotNil(t, svc)

	tpl := env.SeedPolicyTemplate("event-int", "v1")
	space := env.CreateSpaceFixture("event-int-space", tpl)
	eventID := uuid.NewString()
	receivedAt := time.Now().UTC().Add(-time.Minute)
	_, err := svc.Apply(context.Background(), event_hotfix.ApplyInput{
		EventID:    eventID,
		EventType:  "policy-update",
		Payload:    map[string]any{"tenant": env.TenantUUID().String(), "spaceId": space.UUID.String()},
		ReceivedAt: receivedAt,
	})
	require.NoError(t, err)

	_, err = svc.Apply(context.Background(), event_hotfix.ApplyInput{
		EventID:    eventID,
		EventType:  "policy-update",
		Payload:    map[string]any{"tenant": env.TenantUUID().String(), "spaceId": space.UUID.String()},
		ReceivedAt: time.Now().UTC(),
	})
	require.ErrorIs(t, err, event_hotfix.ErrDuplicateEvent)

	oldID := uuid.NewString()
	_, err = svc.Apply(context.Background(), event_hotfix.ApplyInput{
		EventID:    oldID,
		EventType:  "policy-update",
		Payload:    map[string]any{"tenant": env.TenantUUID().String(), "spaceId": space.UUID.String()},
		ReceivedAt: time.Now().UTC().Add(-10 * time.Minute),
	})
	require.ErrorIs(t, err, event_hotfix.ErrInvalidEvent)

	_, err = svc.Retry(context.Background(), event_hotfix.ApplyInput{
		EventID:    oldID,
		EventType:  "policy-update",
		Payload:    map[string]any{"tenant": env.TenantUUID().String(), "spaceId": space.UUID.String()},
		ReceivedAt: time.Now().UTC(),
		RetryCount: 1,
	})
	require.NoError(t, err)

	_, err = svc.Apply(context.Background(), event_hotfix.ApplyInput{
		EventID:    uuid.NewString(),
		EventType:  "agent.weight.refresh",
		Payload:    map[string]any{"targetEventType": "policy-update"},
		ReceivedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	reqPath := env.EventReportPath
	require.FileExists(t, reqPath)
	content, err := os.ReadFile(reqPath)
	require.NoError(t, err)
	var report map[string]any
	require.NoError(t, json.Unmarshal(content, &report))
	require.NotEmpty(t, report["eventType"])
	require.GreaterOrEqual(t, int(report["latencyMs"].(float64)), 0)
	require.LessOrEqual(t, int(report["latencyMs"].(float64)), 5*60*1000)

	var auditCount int64
	require.NoError(t, env.DB.Model(&models.AuditTrailEntry{}).
		Where("space_uuid = ? AND action = ?", space.UUID, "event.hot_update").
		Count(&auditCount).Error)
	require.GreaterOrEqual(t, auditCount, int64(1))
}
