package ops

import (
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
)

// BackupArtifact 记录备份产物元数据。
type BackupArtifact struct {
	coremodel.PowerUUIDModel

	JobID       uint64 `gorm:"column:job_id;not null;index:idx_ops_backup_artifact_job" json:"job_id"`
	StorageURI  string `gorm:"column:storage_uri;type:varchar(512);not null" json:"storage_uri"`
	SizeBytes   int64  `gorm:"column:size_bytes;not null" json:"size_bytes"`
	Checksum    string `gorm:"column:checksum;type:varchar(128);not null" json:"checksum"`
	ContentType string `gorm:"column:content_type;type:varchar(128)" json:"content_type,omitempty"`
}

func (BackupArtifact) TableName() string {
	return coremodel.PowerXSchema + "." + coremodel.TableOpsBackupArtifacts
}

func (m *BackupArtifact) Normalize() {
	m.StorageURI = strings.TrimSpace(m.StorageURI)
	m.Checksum = strings.TrimSpace(strings.ToLower(m.Checksum))
	m.ContentType = strings.TrimSpace(strings.ToLower(m.ContentType))
}
