package backup_ops

import modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"

// ID 0 是按需文件的保留分组，不写入策略表，也不参与定时调度。
func oneTimeDatabasePolicy() *modelops.BackupPolicy {
	return &modelops.BackupPolicy{Name: "按需数据库备份", BackupType: modelops.BackupTypeLogical, RetentionMode: "age_and_count", RetentionDays: 7, RetentionCount: 168}
}
