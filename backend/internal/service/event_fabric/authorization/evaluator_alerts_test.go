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

func TestAuthorizationAlerts(t *testing.T) {
	t.Run("policy missing triggers alert", func(t *testing.T) {
		env := newTestEnv(t)
		ctx := context.Background()

		req := EvaluateRequest{
			TenantID:    uuid.New(),
			SubjectType: SubjectTypeAgent,
			SubjectID:   uuid.New(),
			Capability:  "event_fabric.publish",
		}
		result, err := env.service.Evaluate(ctx, req)
		require.NoError(t, err)
		require.Equal(t, DecisionBlock, result.Decision)

		events := env.alerts.snapshot()
		require.Len(t, events, 1)
		require.Equal(t, "authorization.policy_missing", events[0].Type)
		require.Equal(t, "high", events[0].Severity)
		require.Equal(t, req.Capability, events[0].Capability)
		require.Equal(t, req.SubjectID.String(), events[0].SubjectID)
	})

	t.Run("policy violation alert", func(t *testing.T) {
		env := newTestEnv(t)
		ctx := context.Background()

		tenantID := uuid.New()
		subjectID := uuid.New()
		cap := env.insertCapability("event_fabric", "consume")
		grant := env.insertGrant(tenantID, subjectID, SubjectTypeAgent, eventfabricmodel.GrantStatusActive, time.Now().Add(time.Hour))
		env.insertGrantCapability(grant.UUID, cap.UUID)
		payload, err := json.Marshal(map[string]any{"context_tags": []string{"prod"}})
		require.NoError(t, err)
		env.insertGrantCondition(grant.UUID, eventfabricmodel.GrantConditionTypeContextTag, datatypes.JSON(payload))

		req := EvaluateRequest{
			TenantID:    tenantID,
			SubjectType: SubjectTypeAgent,
			SubjectID:   subjectID,
			Capability:  "event_fabric.consume",
			ContextTags: []string{"staging"},
		}
		result, err := env.service.Evaluate(ctx, req)
		require.NoError(t, err)
		require.Equal(t, DecisionBlock, result.Decision)

		events := env.alerts.snapshot()
		require.Len(t, events, 1)
		require.Equal(t, "authorization.policy_violation", events[0].Type)
		require.Equal(t, "medium", events[0].Severity)
		require.Contains(t, events[0].Reason, "missing required context tag")
	})

	t.Run("challenge required alert", func(t *testing.T) {
		env := newTestEnv(t)
		ctx := context.Background()

		tenantID := uuid.New()
		subjectID := uuid.New()
		cap := env.insertCapability("event_fabric", "publish")
		grant := env.insertGrant(tenantID, subjectID, SubjectTypeAgent, eventfabricmodel.GrantStatusPending, time.Now().Add(time.Hour))
		env.insertGrantCapability(grant.UUID, cap.UUID)
		ticket := env.insertTicket(grant.UUID, eventfabricmodel.ApprovalStatusPending, time.Now().Add(10*time.Minute))

		req := EvaluateRequest{
			TenantID:    tenantID,
			SubjectType: SubjectTypeAgent,
			SubjectID:   subjectID,
			Capability:  "event_fabric.publish",
		}
		result, err := env.service.Evaluate(ctx, req)
		require.NoError(t, err)
		require.Equal(t, DecisionChallenge, result.Decision)
		require.NotNil(t, result.Challenge)
		require.Equal(t, ticket.UUID, result.Challenge.TicketID)

		events := env.alerts.snapshot()
		require.Len(t, events, 1)
		require.Equal(t, "authorization.challenge_required", events[0].Type)
		require.Equal(t, ticket.UUID.String(), events[0].Metadata["ticket_id"])
	})

	t.Run("rate limit exceeded alert", func(t *testing.T) {
		env := newTestEnv(t)
		ctx := context.Background()

		tenantID := uuid.New()
		subjectID := uuid.New()
		cap := env.insertCapability("event_fabric", "publish")
		payload := datatypes.JSON([]byte(`{"limit":1,"burst":1,"interval_seconds":60}`))
		require.NoError(t, env.db.Model(&cap).Update("default_rate_limit", payload).Error)
		grant := env.insertGrant(tenantID, subjectID, SubjectTypeAgent, eventfabricmodel.GrantStatusActive, time.Now().Add(time.Hour))
		env.insertGrantCapability(grant.UUID, cap.UUID)

		env.rateLimiter.setResponse(RateLimitResult{
			Allowed:    false,
			Remaining:  0,
			ResetAfter: time.Minute,
		}, nil)

		req := EvaluateRequest{
			TenantID:    tenantID,
			SubjectType: SubjectTypeAgent,
			SubjectID:   subjectID,
			Capability:  "event_fabric.publish",
		}
		result, err := env.service.Evaluate(ctx, req)
		require.NoError(t, err)
		require.Equal(t, DecisionBlock, result.Decision)
		require.Equal(t, "rate limit exceeded", result.Reason)

		events := env.alerts.snapshot()
		require.Len(t, events, 1)
		require.Equal(t, "authorization.rate_limited", events[0].Type)
		require.Equal(t, "medium", events[0].Severity)
		require.Equal(t, "0", events[0].Metadata["remaining"])
	})
}
