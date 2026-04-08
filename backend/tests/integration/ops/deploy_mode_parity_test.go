package opsintegration

import (
	"context"
	"fmt"
	"testing"

	deployops "github.com/ArtisanCloud/PowerX/internal/service/deploy_ops"
	"github.com/stretchr/testify/require"
)

func TestDeployModeParity(t *testing.T) {
	db := setupDeployDB(t)
	svc := deployops.NewService(db)
	ctx := context.Background()

	cases := []struct {
		name string
		mode string
	}{
		{name: "docker", mode: deployops.DeployModeDocker},
		{name: "systemd", mode: deployops.DeployModeSystemd},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			version := fmt.Sprintf("v2.0.%d", i+1)
			_, err := svc.TriggerRelease(ctx, deployops.ReleaseRequest{
				Environment:     "staging",
				BackendVersion:  version,
				WebAdminVersion: version,
				Mode:            tc.mode,
				Operator:        "integration",
				TraceID:         "trace-mode-" + tc.name,
			})
			require.NoError(t, err)

			_, err = svc.TriggerRollback(ctx, deployops.RollbackRequest{
				Environment:   "staging",
				TargetVersion: version,
				Mode:          tc.mode,
				Operator:      "integration",
				TraceID:       "trace-mode-rb-" + tc.name,
			})
			require.NoError(t, err)
		})
	}

	items, total, err := svc.ListReleases(ctx, deployops.ListReleaseOptions{Environment: "staging", Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.EqualValues(t, 4, total)
	require.Len(t, items, 4)
}
