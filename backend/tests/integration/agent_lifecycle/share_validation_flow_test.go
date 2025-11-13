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

func TestAgentShareValidationFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	ctx := context.Background()
	svc := env.Deps.AgentLifecycle.Service
	agentID := registerTestAgent(t, svc, "tenant-share-src", "share-flow-agent")

	issuedCh := make(chan map[string]any, 1)
	revokedCh := make(chan map[string]any, 1)

	unsubIssued := env.Bus.Subscribe("agent.share.issued", captureShareEvent(issuedCh))
	t.Cleanup(unsubIssued)
	unsubRevoked := env.Bus.Subscribe("agent.share.revoked", captureShareEvent(revokedCh))
	t.Cleanup(unsubRevoked)

	input := agent_lifecycle.ShareInput{
		AgentID:     agentID,
		TenantID:    "tenant-target-alpha",
		Quotas:      []agent_lifecycle.ShareQuota{{Type: "rpm", Limit: 500}},
		Metadata:    map[string]string{"region": "ap-sg"},
		RequestedBy: "ops-share",
		TraceID:     "trace-share-flow",
	}
	share, err := svc.ShareAgent(ctx, input)
	require.NoError(t, err)
	require.Equal(t, "tenant-target-alpha", share.TenantID)
	require.Equal(t, "active", share.Status)
	require.Equal(t, 1, env.QuotaProvisioner.ProvisionCalls)
	require.Len(t, env.ShareValidator.Calls, 1)

	call := env.ShareValidator.Calls[0]
	require.Equal(t, input.TenantID, call.TenantID)
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
