package backup_ops

import (
	"context"
	"fmt"
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// cleanupControlled 在独立提交删除意图后逐产物重新过版本守卫；未知结果不能自动重做。
func (s *ArtifactCleanupService) cleanupControlled(ctx context.Context, policy *model.BackupPolicy) (*CleanupResult, error) {
	root, e := resolveBackupArtifactBaseDir()
	if e != nil {
		return nil, e
	}
	jobs, e := s.jobRepo.ListByPolicyAndStatus(ctx, policy.ID, model.BackupJobStatusSuccess, 0)
	if e != nil {
		return nil, e
	}
	out := &CleanupResult{}
	index := 0
	for _, job := range jobs {
		artifacts, e := s.artifactRepo.ListByJobID(ctx, job.ID)
		if e != nil {
			return out, e
		}
		if len(artifacts) == 0 {
			continue
		}
		expired := backupExpired(job, index, *policy, time.Now().UTC())
		index++
		if !expired {
			continue
		}
		for _, artifact := range artifacts {
			if artifact.CleanupState != "" {
				return out, controlFailure(409, "BACKUP_RETENTION_UNCONFIRMED")
			}
			verified, verifyErr := verifyControlledArtifact(ctx, root, &artifact)
			if verifyErr != nil {
				return out, verifyErr
			}
			token := uuid.NewString()
			e = s.authority.Guard(ctx, policy.ID, policy.ControlRevision, "cleanup", func(run context.Context, tx *gorm.DB, _ *model.BackupPolicy) error {
				result := tx.WithContext(run).Model(&model.BackupArtifact{}).Where("id = ? AND cleanup_state = ?", artifact.ID, "").Updates(map[string]any{"cleanup_state": "pending", "cleanup_token": token})
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return controlFailure(409, "BACKUP_RETENTION_UNCONFIRMED")
				}
				return nil
			})
			if e != nil {
				return out, e
			}
			started := false
			s.notifyChanged()
			e = s.authority.Guard(ctx, policy.ID, policy.ControlRevision, "cleanup", func(run context.Context, tx *gorm.DB, _ *model.BackupPolicy) error {
				var current model.BackupArtifact
				if err := tx.WithContext(run).Where("id = ? AND cleanup_state = ? AND cleanup_token = ?", artifact.ID, "pending", token).First(&current).Error; err != nil {
					return err
				}
				path, err := controlledArtifactRemovalPath(root, &current, verified)
				if err != nil {
					return err
				}
				if run.Err() != nil {
					return run.Err()
				}
				started = true
				if err = removeArtifactFiles(path, &current); err != nil {
					return err
				}
				if err = tx.WithContext(run).Model(&current).Update("cleanup_state", "deleted").Error; err != nil {
					return err
				}
				return tx.WithContext(run).Delete(&current).Error
			})
			if e != nil {
				state := "unconfirmed"
				nextToken := token
				if !started {
					state = ""
					nextToken = ""
				}
				bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
				_ = s.db.WithContext(bounded).Model(&model.BackupArtifact{}).Where("id = ? AND cleanup_state = ? AND cleanup_token = ?", artifact.ID, "pending", token).Updates(map[string]any{"cleanup_state": state, "cleanup_token": nextToken}).Error
				cancel()
				s.notifyChanged()
				return out, fmt.Errorf("backup retention stopped, inspect intent %s: %w", token, e)
			}
			out.DeletedArtifacts++
			s.notifyChanged()
		}
	}
	return out, nil
}
