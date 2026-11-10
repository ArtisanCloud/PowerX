package system_recovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	storagerecovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCurrentStorageBarrierPersistsAcrossReopenAndCompensation(t *testing.T) {
	r, _, _ := currentFixture(t)
	ctx := context.Background()
	require.Equal(t, storagerecovery.Protocol, r.spec.StorageProtocol)
	j, e := r.Coordinator.Store.EnqueueInstanceRestore(privileged(ctx), currentReadySnapshot(t, r), uuid.NewString(), 1)
	require.NoError(t, e)
	require.NoError(t, r.beginStorageRestore(ctx, &j))
	before, e := r.StorageRecoveryAnchor().Read(ctx)
	require.NoError(t, e)
	require.Equal(t, j.ID, before.JobID)
	require.NoError(t, r.beginStorageRestore(ctx, &j))
	after, e := r.StorageRecoveryAnchor().Read(ctx)
	require.NoError(t, e)
	require.Equal(t, before.Epoch, after.Epoch)
	// 没有触碰数据库的补偿不能清除独立保护，即使业务／运行记录已经恢复旧副本。
	require.NoError(t, r.compensateCurrent(ctx, &j))
	reopen, e := openCurrent(r.spec.Directory)
	require.NoError(t, e)
	defer reopen.Close()
	defer reopen.Coordinator.SnapshotStore.Close()
	after, e = reopen.StorageRecoveryAnchor().Read(ctx)
	require.NoError(t, e)
	require.Equal(t, before, after)
	job, e := r.Coordinator.Store.Get(j.ID)
	require.NoError(t, e)
	require.Contains(t, job.Checks, snapshots.RecoveryCheck{Name: "storage_execution_barrier", Passed: true, Detail: "已提交独立恢复代次 " + before.Epoch + "；受控清理和计划保持暂停，恢复后须 Root 核对策略并另行确认"})
}

func TestCurrentStorageProtocolUpgradeRetainsAnchorAndEpoch(t *testing.T) {
	for _, oldProtocol := range []string{storagerecovery.LegacyProtocol, storagerecovery.IntermediateProtocol, storagerecovery.OlderProtocol, storagerecovery.PreviousProtocol, storagerecovery.ImportProtocol, storagerecovery.ResolutionProtocol} {
		t.Run(oldProtocol, func(t *testing.T) {
			r, _, _ := currentFixture(t)
			before, e := r.StorageRecoveryAnchor().Begin(context.Background(), uuid.NewString(), uuid.NewString())
			require.NoError(t, e)
			for _, file := range []string{"storage-recovery-binding.json", "storage-recovery-epoch.json"} {
				path := filepath.Join(r.spec.Directory, file)
				raw, e := os.ReadFile(path)
				require.NoError(t, e)
				var doc map[string]any
				require.NoError(t, json.Unmarshal(raw, &doc))
				doc["protocol"] = oldProtocol
				require.NoError(t, snapshots.WriteCoordinatorJSON(path, doc))
			}
			r.spec.StorageProtocol = oldProtocol
			require.NoError(t, snapshots.WriteCoordinatorJSON(filepath.Join(r.spec.Directory, "installation.json"), r.spec))
			reopened, e := openCurrent(r.spec.Directory)
			require.NoError(t, e)
			defer reopened.Close()
			defer reopened.Coordinator.SnapshotStore.Close()
			require.Equal(t, storagerecovery.Protocol, reopened.spec.StorageProtocol)
			after, e := reopened.StorageRecoveryAnchor().Read(context.Background())
			require.NoError(t, e)
			require.Equal(t, before, after)
			require.NoError(t, os.Remove(filepath.Join(r.spec.Directory, "storage-recovery-epoch.json")))
			_, e = openCurrent(r.spec.Directory)
			require.Error(t, e, "never reinitialize proof already marked as upgraded")
		})
	}
}
func TestCurrentStorageRequiredMarkerRejectsMissingAnchor(t *testing.T) {
	r, _, _ := currentFixture(t)
	require.NoError(t, os.Remove(filepath.Join(r.spec.Directory, "storage-recovery-epoch.json")))
	_, e := openCurrent(r.spec.Directory)
	require.Error(t, e)
}
