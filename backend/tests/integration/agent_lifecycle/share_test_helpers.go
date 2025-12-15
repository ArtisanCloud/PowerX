//go:build ignore

package agentlifecycleintegration

import (
	"context"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func registerTestAgent(t *testing.T, svc *agent_lifecycle.Service, tenantUUID, alias string) uuid.UUID {
	t.Helper()
	res, err := svc.Register(context.Background(), agent_lifecycle.RegisterInput{
		TenantUUID:               tenantUUID,
		Alias:                    alias,
		TelemetryContractVersion: "otel-agent-v1",
	})
	require.NoError(t, err)
	require.NotNil(t, res.Agent)
	return res.Agent.ID
}
