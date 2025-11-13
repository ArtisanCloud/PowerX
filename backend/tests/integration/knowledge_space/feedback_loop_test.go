package knowledge_space_integration

import (
	"context"
	"encoding/json"
	"testing"

	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestFeedbackLoopSchedulesReprocess(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	ctx := context.Background()
	policyID := env.SeedPolicyTemplate("feedback-integration", "v1")
	space := env.CreateSpaceFixture("feedback-space", policyID)

	svc := env.Deps.KnowledgeSpace.Feedback
	require.NotNil(t, svc)

	chunkID := uuid.New()
	caseModel, err := svc.SubmitFeedback(ctx, ksvc.SubmitFeedbackInput{
		SpaceID:      space.UUID,
		ReportedBy:   "qa@powerx.local",
		Severity:     "medium",
		IssueType:    "freshness",
		LinkedChunks: []uuid.UUID{chunkID},
		Notes:        "内容已经过期",
	})
	require.NoError(t, err)
	require.NotNil(t, caseModel)
	require.Equal(t, models.FeedbackStatusInProgress, caseModel.Status)
	require.NotNil(t, caseModel.ReprocessJobID)

	require.NotNil(t, env.Pipeline)
	last := env.Pipeline.LastInput()
	require.Equal(t, space.UUID, last.SpaceID)
	require.Equal(t, caseModel.UUID, last.CaseID)
	require.Equal(t, chunkID, last.ChunkIDs[0])

	var storedChunks []string
	require.NoError(t, json.Unmarshal(caseModel.LinkedChunks, &storedChunks))
	require.Contains(t, storedChunks, chunkID.String())
}
