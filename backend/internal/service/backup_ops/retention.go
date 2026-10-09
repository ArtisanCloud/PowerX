package backup_ops

import (
	"strings"
	"time"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
)

func normalizeRetention(mode string, days int) (string, int, error) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "age_and_count"
	}
	if mode != "count" && mode != "age_and_count" {
		return "", 0, ErrInvalidBackupPolicy
	}
	if days == 0 {
		days = 7
	}
	if days < 1 || days > 3650 {
		return "", 0, ErrInvalidBackupPolicy
	}
	return mode, days, nil
}

func backupExpired(job modelops.BackupJob, index int, policy modelops.BackupPolicy, now time.Time) bool {
	if job.Protected || index == 0 {
		return false
	}
	count := int(policy.RetentionCount)
	if count < 1 {
		count = defaultRetentionCount
	}
	if index >= count {
		return true
	}
	if policy.RetentionMode != "age_and_count" || policy.RetentionDays < 1 {
		return false
	}
	created := job.CreatedAt
	if job.EndedAt != nil {
		created = *job.EndedAt
	}
	return !created.IsZero() && created.Before(now.Add(-time.Duration(policy.RetentionDays)*24*time.Hour))
}
