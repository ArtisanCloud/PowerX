//go:build ignore

package agentlifecycleintegration

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/ArtisanCloud/PowerX/tests/agent_lifecycle/testenv"
	"github.com/stretchr/testify/require"
)

const (
	shareFlowSourceUUID = "33a3fbb0-86c4-4ec8-bc3c-765b8e3228be"
	shareFlowTargetUUID = "b61fa815-32bd-4cb5-86b8-998b4da4c5e2"
)

func TestAgentShareValidationFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	ctx := context.Background()
	svc := env.Deps.AgentLifecycle.Service
	agentID := registerTestAgent(t, svc, shareFlowSourceUUID, "share-flow-agent")

	issuedCh := make(chan map[string]any, 1)
	revokedCh := make(chan map[string]any, 1)

	unsubIssued := env.Bus.Subscribe("agent.share.issued", captureShareEvent(issuedCh))
	t.Cleanup(unsubIssued)
	unsubRevoked := env.Bus.Subscribe("agent.share.revoked", captureShareEvent(revokedCh))
	t.Cleanup(unsubRevoked)

	input := agent_lifecycle.ShareInput{
		AgentID:     agentID,
		TenantUUID:  shareFlowTargetUUID,
		Quotas:      []agent_lifecycle.ShareQuota{{Type: "rpm", Limit: 500}},
		Metadata:    map[string]string{"region": "ap-sg"},
		RequestedBy: "ops-share",
		TraceID:     "trace-share-flow",
	}
	share, err := svc.ShareAgent(ctx, input)
	require.NoError(t, err)
	require.Equal(t, shareFlowTargetUUID, share.TenantUUID)
	require.Equal(t, "active", share.Status)
	require.Equal(t, 1, env.QuotaProvisioner.ProvisionCalls)
	require.Len(t, env.ShareValidator.Calls, 1)

	call := env.ShareValidator.Calls[0]
	require.Equal(t, input.TenantUUID, call.TenantUUID)
	require.Equal(t, agentID, call.AgentID)
	require.Equal(t, input.Metadata["region"], call.Metadata["region"])
	require.Len(t, call.Quotas, 1)

	select {
	case payload := <-issuedCh:
		require.Equal(t, share.ID.String(), payload["share_id"])
		require.Equal(t, "active", payload["status"])
	case <-time.After(2 * time.Second):
		t.Fatalf("expected agent.share.issued event")
	}

	revokeInput := agent_lifecycle.RevokeShareInput{
		ShareID:     share.ID,
		Reason:      "lease-expired",
		RequestedBy: "ops-share",
		TraceID:     "trace-share-revoke",
	}
	revokedShare, err := svc.RevokeAgentShare(ctx, revokeInput)
	require.NoError(t, err)
	require.Equal(t, "revoked", revokedShare.Status)
	require.Equal(t, 1, env.QuotaProvisioner.ReleaseCalls)

	select {
	case payload := <-revokedCh:
		require.Equal(t, share.ID.String(), payload["share_id"])
		require.Equal(t, "revoked", payload["status"])
	case <-time.After(2 * time.Second):
		t.Fatalf("expected agent.share.revoked event")
	}
}

func captureShareEvent(ch chan<- map[string]any) event_bus.Handler {
	return func(evt event_bus.Event) error {
		if payload, ok := evt.Payload.(map[string]any); ok {
			select {
			case ch <- payload:
			default:
			}
		}
		return nil
	}
}
