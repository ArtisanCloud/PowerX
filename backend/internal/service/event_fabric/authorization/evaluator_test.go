package authorization

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestServiceEvaluate(t *testing.T) {
	t.Run("allow grant cached", func(t *testing.T) {
		env := newTestEnv(t)
		ctx := context.Background()

		tenantID := uuid.New()
		subjectID := uuid.New()
		cap := env.insertCapability("event_fabric", "publish")

		grant := env.insertGrant(tenantID, subjectID, SubjectTypeAgent, eventfabricmodel.GrantStatusActive, time.Now().Add(30*time.Minute))
		env.insertGrantCapability(grant.UUID, cap.UUID)

		req := EvaluateRequest{
			TenantID:    tenantID,
			SubjectType: SubjectTypeAgent,
			SubjectID:   subjectID,
			Capability:  "event_fabric.publish",
			Resource:    "topic://payments",
		}

		result, err := env.service.Evaluate(ctx, req)
		require.NoError(t, err)
		require.Equal(t, DecisionAllow, result.Decision)
		require.False(t, result.CacheHit)
		require.EqualValues(t, grant.Version, result.GrantVersion)

		result2, err := env.service.Evaluate(ctx, req)
		require.NoError(t, err)
		require.Equal(t, DecisionAllow, result2.Decision)
		require.True(t, result2.CacheHit)
		require.Equal(t, result.GrantVersion, result2.GrantVersion)
	})

	t.Run("block when resource not allowed", func(t *testing.T) {
		env := newTestEnv(t)
		ctx := context.Background()

		tenantID := uuid.New()
		subjectID := uuid.New()
		cap := env.insertCapability("event_fabric", "consume")

		grant := env.insertGrant(tenantID, subjectID, SubjectTypePlugin, eventfabricmodel.GrantStatusActive, time.Now().Add(30*time.Minute))
		env.insertGrantCapability(grant.UUID, cap.UUID)

		payload, err := json.Marshal(map[string]any{
			"resources": []string{"topic://allowed"},
		})
		require.NoError(t, err)
		env.insertGrantCondition(grant.UUID, eventfabricmodel.GrantConditionTypeResource, datatypes.JSON(payload))

		req := EvaluateRequest{
			TenantID:    tenantID,
			SubjectType: SubjectTypePlugin,
			SubjectID:   subjectID,
			Capability:  "event_fabric.consume",
			Resource:    "topic://denied",
		}

		result, err := env.service.Evaluate(ctx, req)
		require.NoError(t, err)
		require.Equal(t, DecisionBlock, result.Decision)
		require.Contains(t, result.Reason, "resource not allowed")
	})

	t.Run("challenge pending ticket", func(t *testing.T) {
		env := newTestEnv(t)
		ctx := context.Background()

		tenantID := uuid.New()
		subjectID := uuid.New()
		cap := env.insertCapability("event_fabric", "publish")

		grant := env.insertGrant(tenantID, subjectID, SubjectTypeAgent, eventfabricmodel.GrantStatusPending, time.Now().Add(30*time.Minute))
		env.insertGrantCapability(grant.UUID, cap.UUID)
		ticket := env.insertTicket(grant.UUID, eventfabricmodel.ApprovalStatusPending, time.Now().Add(10*time.Minute))

		req := EvaluateRequest{
			TenantID:    tenantID,
			SubjectType: SubjectTypeAgent,
			SubjectID:   subjectID,
			Capability:  "event_fabric.publish",
			Resource:    "topic://payments",
		}

		result, err := env.service.Evaluate(ctx, req)
		require.NoError(t, err)
		require.Equal(t, DecisionChallenge, result.Decision)
		require.NotNil(t, result.Challenge)
		require.Equal(t, ticket.UUID.String(), result.Challenge.TicketID.String())
		require.WithinDuration(t, ticket.SLAExpiresAt, result.Challenge.SLAExpiresAt, time.Second)
	})
}
