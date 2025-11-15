package knowledge_space_integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	tenant_release "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/tenant_release"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/stretchr/testify/require"
)

func TestTenantReleaseFlow(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	svc := env.Deps.KnowledgeSpace.Release
	require.NotNil(t, svc)

	policy, err := svc.UpsertPolicy(context.Background(), tenant_release.UpsertPolicyInput{
		MatrixVersion: "v2025.02",
		PilotTenants:  []string{"demo-retail"},
		Batches: []tenant_release.BatchSpec{
			{Name: "pilot", Tenants: []string{"demo-retail"}},
			{Name: "wave-2", Tenants: []string{"demo-lite", "demo-enterprise"}},
		},
		Guardrails: map[string]string{"latency_p95": "<5m"},
		ApprovedBy: "ops@powerx.io",
	})
	require.NoError(t, err)

	publishRes, err := svc.Publish(context.Background(), tenant_release.PublishInput{
		PolicyID:    policy.ID,
		VersionID:   "ver-2025.02",
		RequestedBy: "qa@powerx.io",
	})
	require.NoError(t, err)
	require.NotEmpty(t, publishRes.BatchToken)

	promoteRes, err := svc.Promote(context.Background(), tenant_release.PromoteInput{
		PolicyID:    policy.ID,
		VersionID:   "ver-2025.02",
		BatchToken:  publishRes.BatchToken,
		RequestedBy: "ops@powerx.io",
	})
	require.NoError(t, err)
	require.Equal(t, "promoted", promoteRes.State)

	_, err = svc.Promote(context.Background(), tenant_release.PromoteInput{
		PolicyID:    policy.ID,
		VersionID:   "ver-2025.02",
		BatchToken:  promoteRes.BatchToken,
		RequestedBy: "ops@powerx.io",
	})
	require.NoError(t, err)

	rollbackRes, err := svc.Rollback(context.Background(), tenant_release.RollbackInput{
		PolicyID:    policy.ID,
		VersionID:   "ver-2025.02",
		Reason:      "metrics breached",
		RequestedBy: "ops@powerx.io",
	})
	require.NoError(t, err)
	require.Equal(t, "rolled_back", rollbackRes.Status)

	data, err := os.ReadFile(env.ReleaseReportPath)
	require.NoError(t, err)
	var snapshot map[string]any
	require.NoError(t, json.Unmarshal(data, &snapshot))
	require.Equal(t, "rolled_back", snapshot["grayState"])
}
