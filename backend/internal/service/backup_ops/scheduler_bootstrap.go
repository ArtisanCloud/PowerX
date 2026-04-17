package backup_ops

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// RegisterPolicyScheduler 在 bootstrap 阶段注册备份策略调度器。
func RegisterPolicyScheduler(ctx context.Context, db *gorm.DB, tick time.Duration) *JobService {
	if db == nil {
		return nil
	}
	svc := NewJobService(db)
	svc.RegisterPolicyScheduler(ctx, tick)
	return svc
}
