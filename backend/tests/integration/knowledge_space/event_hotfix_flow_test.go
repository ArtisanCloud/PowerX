package knowledge_space_integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	event_hotfix "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/event_hotfix"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestEventHotfixFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	svc := env.Deps.KnowledgeSpace.EventHotfix
	require.NotNil(t, svc)
	eventID := uuid.NewString()
	_, err := svc.Apply(context.Background(), event_hotfix.ApplyInput{
		EventID:    eventID,
		EventType:  "policy-update",
		Payload:    map[string]any{"tenant": env.TenantID().String()},
		ReceivedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	reqPath := env.EventReportPath
	require.FileExists(t, reqPath)
	content, err := os.ReadFile(reqPath)
	require.NoError(t, err)
	var report map[string]any
	require.NoError(t, json.Unmarshal(content, &report))
	require.Equal(t, eventID, report["eventId"])
}
