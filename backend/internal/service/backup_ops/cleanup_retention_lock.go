package backup_ops

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/binary"
	"gorm.io/gorm"
	"hash/fnv"
	"time"
)

// One dedicated session holds request and the existing policy advisory lock, leaving SQL work on a second connection.
func lockCleanupRetention(ctx context.Context, db *gorm.DB, request string) (func(context.Context, uint64) error, func(), error) {
	pool, e := db.DB()
	if e != nil {
		return nil, nil, e
	}
	conn, e := pool.Conn(ctx)
	if e != nil {
		return nil, nil, e
	}
	var database string
	if e = conn.QueryRowContext(ctx, "SELECT current_database()").Scan(&database); e != nil {
		conn.Close()
		return nil, nil, e
	}
	sum := sha256.Sum256([]byte("powerx.backup.retain/v1/" + database + "/" + request))
	key := int64(binary.BigEndian.Uint64(sum[:8]))
	var locked bool
	if e = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&locked); e != nil || !locked {
		conn.Close()
		if e != nil {
			return nil, nil, e
		}
		return nil, nil, controlFailure(409, "BACKUP_RETAIN_REQUEST_BUSY")
	}
	h := fnv.New32a()
	h.Write([]byte("powerx.backup." + database))
	class := int32(h.Sum32())
	var policy int32
	hasPolicy := false
	take := func(parent context.Context, id uint64) error {
		run, cancel := context.WithTimeout(parent, 3*time.Second)
		defer cancel()
		var yes bool
		e := conn.QueryRowContext(run, "SELECT pg_try_advisory_lock($1,$2)", class, int32(id)).Scan(&yes)
		if e != nil {
			return e
		}
		if !yes {
			return ErrBackupJobAlreadyRunning
		}
		policy = int32(id)
		hasPolicy = true
		return nil
	}
	release := func() {
		run, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		ok := true
		var yes bool
		if hasPolicy {
			e := conn.QueryRowContext(run, "SELECT pg_advisory_unlock($1,$2)", class, policy).Scan(&yes)
			ok = e == nil && yes
		}
		e := conn.QueryRowContext(run, "SELECT pg_advisory_unlock($1)", key).Scan(&yes)
		if e != nil || !yes || !ok {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		_ = conn.Close()
	}
	return take, release, nil
}
