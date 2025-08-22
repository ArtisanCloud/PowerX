// pkg/corex/audit/janitor.go
package audit

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type JanitorCfg struct {
	Retention time.Duration // 例如 90 * 24h
	BatchSize int           // 10000
	Every     time.Duration // 例如 24h
	LeaseKey  string        // 防多实例同时清理，可选
}

func StartJanitor(ctx context.Context, db *gorm.DB, cfg JanitorCfg) {
	if cfg.Retention <= 0 {
		return
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 10000
	}
	if cfg.Every <= 0 {
		cfg.Every = 24 * time.Hour
	}

	t := time.NewTicker(3 * time.Second) // 首次延迟很短
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			cutoff := time.Now().Add(-cfg.Retention)
			for {
				res := db.Exec(`
					WITH del AS (
					  SELECT id FROM public.audit_event
					  WHERE occurred_at < @cutoff
					  ORDER BY id
					  LIMIT @limit
					)
					DELETE FROM public.audit_event a USING del
					WHERE a.id = del.id`,
					map[string]any{"cutoff": cutoff, "limit": cfg.BatchSize})
				if res.Error != nil {
					break
				}
				if res.RowsAffected == 0 {
					break
				}
				// 小睡避免占满 IO
				time.Sleep(200 * time.Millisecond)
			}
			// 正常下一次执行
			t.Reset(cfg.Every)
		}
	}
}
