package system_recovery

import (
	"context"
	"errors"
	"fmt"
	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"path/filepath"
	"time"

	storagerecovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
)

func (r *CurrentRuntime) openStorageAnchor() error {
	var err error
	if r.spec.StorageProtocol == "" {
		r.storageAnchor, err = storagerecovery.Initialize(r.spec.Directory, r.spec.ID)
		if err != nil {
			return err
		}
		r.spec.StorageProtocol = storagerecovery.Protocol
		return snapshots.WriteCoordinatorJSON(filepath.Join(r.spec.Directory, "installation.json"), r.spec)
	}
	if r.spec.StorageProtocol == storagerecovery.ResolutionProtocol || r.spec.StorageProtocol == storagerecovery.ImportProtocol || r.spec.StorageProtocol == storagerecovery.PreviousProtocol || r.spec.StorageProtocol == storagerecovery.OlderProtocol || r.spec.StorageProtocol == storagerecovery.IntermediateProtocol || r.spec.StorageProtocol == storagerecovery.LegacyProtocol {
		r.storageAnchor, err = storagerecovery.UpgradeV1(r.spec.Directory, r.spec.ID)
		if err != nil {
			return err
		}
		r.spec.StorageProtocol = storagerecovery.Protocol
		return snapshots.WriteCoordinatorJSON(filepath.Join(r.spec.Directory, "installation.json"), r.spec)
	}
	if r.spec.StorageProtocol != storagerecovery.Protocol {
		return errors.New("存储恢复保护协议不匹配")
	}
	r.storageAnchor, err = storagerecovery.Open(r.spec.Directory, r.spec.ID)
	return err
}

// StorageRecoveryAnchor 复用现有维护目录，恢复包及业务回滚都不能覆盖它。
func (r *CurrentRuntime) StorageRecoveryAnchor() *storagerecovery.Store { return r.storageAnchor }

func (r *CurrentRuntime) beginStorageRestore(ctx context.Context, j *snapshots.ManagedJob) error {
	return r.beginStorageRestoreChecked(ctx, j, "")
}
func (r *CurrentRuntime) beginStorageRestoreChecked(ctx context.Context, j *snapshots.ManagedJob, dsn string) error {
	if r.storageAnchor == nil {
		return errors.New("存储恢复保护不可用，未覆盖当前数据")
	}
	covered := 0
	var check func(map[string]storagerecovery.RetentionRecord) error
	if dsn != "" {
		check = func(records map[string]storagerecovery.RetentionRecord) error {
			bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
			if e != nil {
				return errors.New("BACKUP_HISTORY_SOURCE_UNAVAILABLE")
			}
			pool, e := db.DB()
			if e != nil {
				return errors.New("BACKUP_HISTORY_SOURCE_UNAVAILABLE")
			}
			defer pool.Close()
			pool.SetMaxOpenConns(1)
			covered, e = backupops.CheckRetentionRecoveryCoverage(bounded, db, coremodel.PowerXSchema, records)
			return e
		}
	}
	epoch, err := r.storageAnchor.BeginChecked(ctx, j.ID, j.SnapshotID, check)
	if err != nil {
		return err
	}
	j.Checks = append(j.Checks, snapshots.RecoveryCheck{Name: "storage_execution_barrier", Passed: true, Detail: "已提交独立恢复代次 " + epoch.Epoch + "；受控清理和计划保持暂停，恢复后须 Root 核对策略并另行确认"})
	j.Checks = append(j.Checks, snapshots.RecoveryCheck{Name: "backup_request_recovery_ledger", Passed: true, Detail: "已在覆盖数据库前核对独立保留请求凭据；恢复后历史只读，不重放保护操作"})
	if check != nil {
		j.Checks = append(j.Checks, snapshots.RecoveryCheck{Name: "backup_request_history_coverage", Passed: true, Detail: fmt.Sprintf("已核对当前数据库 %d 条请求身份的独立覆盖；不是当前文件或保护状态证明", covered)})
	}
	return r.Coordinator.Store.Save(*j)
}
