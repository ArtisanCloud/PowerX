package backup_ops

import (
	repoops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"gorm.io/gorm"
)

type BackupPolicyRepository = repoops.BackupPolicyRepository
type BackupJobRepository = repoops.BackupJobRepository
type BackupArtifactRepository = repoops.BackupArtifactRepository
type RestoreDrillRecordRepository = repoops.RestoreDrillRecordRepository
type BackupAlertRepository = repoops.BackupAlertRepository

func NewBackupPolicyRepository(db *gorm.DB) *BackupPolicyRepository {
	return repoops.NewBackupPolicyRepository(db)
}

func NewBackupJobRepository(db *gorm.DB) *BackupJobRepository {
	return repoops.NewBackupJobRepository(db)
}

func NewBackupArtifactRepository(db *gorm.DB) *BackupArtifactRepository {
	return repoops.NewBackupArtifactRepository(db)
}

func NewRestoreDrillRecordRepository(db *gorm.DB) *RestoreDrillRecordRepository {
	return repoops.NewRestoreDrillRecordRepository(db)
}

func NewBackupAlertRepository(db *gorm.DB) *BackupAlertRepository {
	return repoops.NewBackupAlertRepository(db)
}
