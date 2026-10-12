package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

var ErrMigrationLocked = errors.New("UPGRADE_LOCK_BUSY")

// WithDatabaseMigrationLock 升级与既有 migrate 共用同一个库/schema 执行锁。
func WithDatabaseMigrationLock(ctx context.Context, db *gorm.DB, timeout time.Duration, apply func(*gorm.DB) error) error {
	return db.WithContext(ctx).Connection(func(conn *gorm.DB) error {
		conn = conn.Session(&gorm.Session{NewDB: true})
		lockName := "powerx:upgrade:" + coremodel.PowerXSchema
		deadline := time.Now().Add(timeout)
		for {
			var locked bool
			if err := conn.Raw("SELECT pg_try_advisory_lock(hashtextextended(current_database() || ?,0))", lockName).Scan(&locked).Error; err != nil {
				return err
			}
			if locked {
				break
			}
			if !time.Now().Before(deadline) {
				return ErrMigrationLocked
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := conn.WithContext(cleanup).Exec("SELECT pg_advisory_unlock(hashtextextended(current_database() || ?,0))", lockName).Error; err != nil {
				if c, ok := conn.Statement.ConnPool.(*sql.Conn); ok {
					_ = c.Raw(func(any) error { return driver.ErrBadConn })
				}
			}
		}()
		return apply(conn)
	})
}
