package backup_ops

import (
	"context"
	"database/sql/driver"
	"hash/fnv"

	"gorm.io/gorm"
)

// 同一 PostgreSQL 实例中按数据库和策略锁定；保护手动、定时、清理、恢复之间的竞争。
func lockBackupPolicy(ctx context.Context, db *gorm.DB, policyID uint64) (func(), error) {
	if db == nil || db.Dialector == nil || db.Dialector.Name() != "postgres" {
		return nil, ErrUnsupportedBackupDatabase
	}
	pool, err := db.DB()
	if err != nil {
		return nil, err
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		return nil, err
	}
	var database string
	if err = conn.QueryRowContext(ctx, "SELECT current_database()").Scan(&database); err != nil {
		conn.Close()
		return nil, err
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte("powerx.backup." + database))
	key, id := int32(h.Sum32()), int32(policyID)
	var locked bool
	if err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1, $2)", key, id).Scan(&locked); err != nil || !locked {
		conn.Close()
		if err != nil {
			return nil, err
		}
		return nil, ErrBackupJobAlreadyRunning
	}
	return func() {
		// 解锁失败时丢弃连接，防止会话锁留在池中。
		var unlocked bool
		if err := conn.QueryRowContext(context.Background(), "SELECT pg_advisory_unlock($1, $2)", key, id).Scan(&unlocked); err != nil || !unlocked {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		_ = conn.Close()
	}, nil
}
