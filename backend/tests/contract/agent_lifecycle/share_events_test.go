//go:build ignore

package agentlifecyclecontract

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/tests/agent_lifecycle/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const (
	shareEventOwnerUUID  = "90a3522a-0fa0-45ae-8d83-5f05ebc9ecdf"
	shareEventTargetUUID = "2a17f6c9-7ad7-46de-aec7-82058f6a0a5d"
)

func TestShareEventsPublished(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	issueCh := make(chan event_bus.Event, 1)
	revokeCh := make(chan event_bus.Event, 1)

	unsubIssue := env.Bus.Subscribe("agent.share.issued", func(evt event_bus.Event) error {
		issueCh <- evt
		return nil
	})
	unsubRevoke := env.Bus.Subscribe("agent.share.revoked", func(evt event_bus.Event) error {
		revokeCh <- evt
		return nil
	})
	t.Cleanup(func() {
		if unsubIssue != nil {
			unsubIssue()
		}
		if unsubRevoke != nil {
			unsubRevoke()
		}
	})

	agentID := env.SeedAgent(shareEventOwnerUUID, "event-share")

	share, err := env.Deps.AgentLifecycle.Service.ShareAgent(context.Background(), agent_lifecycle.ShareInput{
		AgentID:     agentID,
		TenantUUID:  shareEventTargetUUID,
		RequestedBy: "ops-events",
		TraceID:     "trace-events-1",
	})
	require.NoError(t, err)

	select {
	case evt := <-issueCh:
		payload, ok := evt.Payload.(map[string]any)
		require.True(t, ok)
		require.Equal(t, share.ID.String(), payload["share_id"])
		require.Equal(t, share.AgentID.String(), payload["agent_id"])
		require.Equal(t, share.TenantUUID, payload["target_tenant_uuid"])
	case <-time.After(2 * time.Second):
		t.Fatal("expected agent.share.issued event")
	}

	revokeShare, err := env.Deps.AgentLifecycle.Service.RevokeAgentShare(context.Background(), agent_lifecycle.RevokeShareInput{
		ShareID:     share.ID,
		Reason:      "policy",
		RequestedBy: "ops-events",
		TraceID:     "trace-events-2",
	})
	require.NoError(t, err)
	require.Equal(t, "revoked", revokeShare.Status)

	select {
	case evt := <-revokeCh:
		payload, ok := evt.Payload.(map[string]any)
		require.True(t, ok)
		require.Equal(t, share.ID.String(), payload["share_id"])
		require.Equal(t, "revoked", payload["status"])
	case <-time.After(2 * time.Second):
		t.Fatal("expected agent.share.revoked event")
	}

	_, err = env.Deps.AgentLifecycle.Service.RevokeAgentShare(context.Background(), agent_lifecycle.RevokeShareInput{
		ShareID: uuid.New(),
	})
	require.Error(t, err)
}
