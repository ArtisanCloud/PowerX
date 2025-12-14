package knowledge_space_integration

import (
	"context"
	"sync"
	"testing"

	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/tests/knowledge_space/testenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestProvisioningFlowEndToEnd(t *testing.T) {
	env := testenv.New(t)
	t.Cleanup(env.Close)

	service := env.Deps.KnowledgeSpace.Service
	require.NotNil(t, service)

	policyID := env.SeedPolicyTemplate("integration-template", "v1")

	ctx := context.Background()
	createInput := ksvc.CreateSpaceInput{
		TenantUUID:     env.TenantUUID().String(),
		SpaceName:      "integration-space",
		DepartmentCode: "RND",
		QuotaCPU:       8,
		QuotaStorageGB: 320,
		PolicyVersion:  policyID,
		FeatureFlags:   []string{"iam.pending.badge"},
		RequestedBy:    "qa.user@powerx.io",
	}

	space, err := service.CreateSpace(ctx, createInput)
	require.NoError(t, err)
	require.Equal(t, models.KnowledgeSpaceStatusPending, space.Status)

	var iamTask models.IAMSyncTask
	require.NoError(t, env.DB.WithContext(ctx).Where("space_uuid = ?", space.UUID).Take(&iamTask).Error)
	require.Equal(t, models.IAMSyncStatusPending, iamTask.Status)

	active, err := service.UpdateSpace(ctx, ksvc.UpdateSpaceInput{
		SpaceID:        space.UUID,
		Status:         models.KnowledgeSpaceStatusActive,
		QuotaCPU:       8,
		QuotaStorageGB: 320,
	})
	require.NoError(t, err)
	require.Equal(t, models.KnowledgeSpaceStatusActive, active.Status)

	parallelName := "integration-" + uuid.NewString()

	errCh := make(chan error, 2)
	wg := sync.WaitGroup{}
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer wg.Done()
			_, err := service.CreateSpace(ctx, ksvc.CreateSpaceInput{
				TenantUUID:     env.TenantUUID().String(),
				SpaceName:      parallelName,
				DepartmentCode: "RND",
				QuotaCPU:       4,
				QuotaStorageGB: 200,
				PolicyVersion:  policyID,
				RequestedBy:    "parallel.tester",
			})
			errCh <- err
		}(i)
	}
	wg.Wait()
	close(errCh)

	conflictCount := 0
	for err := range errCh {
		if err == nil {
			continue
		}
		if ksvc.IsConflictError(err) {
			conflictCount++
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	require.Equal(t, 1, conflictCount, "one of the concurrent creates should be rejected with conflict")
}
