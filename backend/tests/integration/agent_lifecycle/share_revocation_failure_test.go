//go:build ignore

package agentlifecycleintegration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/ArtisanCloud/PowerX/tests/agent_lifecycle/testenv"
	"github.com/stretchr/testify/require"
)

const (
	shareSourceUUID    = "c67b02b9-3cd2-4f8e-92a8-9b65c8e7e9d7"
	shareTargetBetaUUID = "ab29a128-6f6c-4b76-8c95-97db2d6f1f6a"
	shareDeniedUUID     = "f8e46642-47f7-44b4-bb4a-1b6e135c8519"
)

func TestAgentShareRevocationFailureFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	ctx := context.Background()
	svc := env.Deps.AgentLifecycle.Service
	agentID := registerTestAgent(t, svc, shareSourceUUID, "share-revoke-agent")

	share, err := svc.ShareAgent(ctx, agent_lifecycle.ShareInput{
		AgentID:     agentID,
		TenantUUID:  shareTargetBetaUUID,
		Quotas:      []agent_lifecycle.ShareQuota{{Type: "rpm", Limit: 300}},
		Metadata:    map[string]string{"region": "eu-west"},
		RequestedBy: "ops-share",
		TraceID:     "trace-revoke-flow",
	})
	require.NoError(t, err)

	env.ShareValidator.Err = errors.New("tenant not allowed")
	_, err = svc.ShareAgent(ctx, agent_lifecycle.ShareInput{
		AgentID:     agentID,
		TenantUUID:  shareDeniedUUID,
		RequestedBy: "ops-share",
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, agent_lifecycle.ErrShareValidationFailed))
	env.ShareValidator.Err = nil

	revokedCh := make(chan map[string]any, 1)
	unsubRevoked := env.Bus.Subscribe("agent.share.revoked", captureShareEvent(revokedCh))
	t.Cleanup(unsubRevoked)

	env.QuotaProvisioner.ErrRelease = errors.New("release failure")
	_, err = svc.RevokeAgentShare(ctx, agent_lifecycle.RevokeShareInput{
		ShareID:     share.ID,
		Reason:      "forced-cleanup",
		RequestedBy: "ops-share",
		TraceID:     "trace-revoke-attempt",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "release failure")

	select {
	case <-revokedCh:
		t.Fatalf("revocation failure should not emit event")
	case <-time.After(200 * time.Millisecond):
	}

	record, dbErr := env.Deps.AgentLifecycle.ShareRepo.GetByUUID(ctx, share.ID)
	require.NoError(t, dbErr)
	require.Equal(t, "active", record.Status)

	require.Equal(t, 1, env.QuotaProvisioner.ReleaseCalls)

	env.QuotaProvisioner.ErrRelease = nil
	finalShare, err := svc.RevokeAgentShare(ctx, agent_lifecycle.RevokeShareInput{
		ShareID:     share.ID,
		Reason:      "forced-cleanup",
		RequestedBy: "ops-share",
		TraceID:     "trace-revoke-success",
	})
	require.NoError(t, err)
	require.Equal(t, "revoked", finalShare.Status)
	require.Equal(t, 2, env.QuotaProvisioner.ReleaseCalls)

	select {
	case payload := <-revokedCh:
		require.Equal(t, share.ID.String(), payload["share_id"])
		require.Equal(t, "revoked", payload["status"])
	case <-time.After(2 * time.Second):
		t.Fatalf("expected revocation event after successful retry")
	}
}
