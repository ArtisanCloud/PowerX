package runtimescheduler

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type localFence struct {
	token chan struct{}
	refs  int
}

var fences = struct {
	sync.Mutex
	values map[string]*localFence
}{values: map[string]*localFence{}}

// WithJobFence holds one job's mutation/publication fence until the callback
// returns. PostgreSQL uses a pinned connection/session lock, so publication
// AFTER transaction commit remains fenced across Core replicas. Session loss
// releases the lock; cleanup failure discards the connection, never pooling it.
func (r *JobRepository) WithJobFence(ctx context.Context, id uuid.UUID, fn func(*gorm.DB) error) error {
	if r.db.Dialector.Name() == "postgres" {
		return r.db.WithContext(ctx).Connection(func(conn *gorm.DB) error {
			key := "powerx.runtime.scheduler:" + id.String()
			defer func() {
				cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
				defer cancel()
				if conn.WithContext(cleanup).Exec("SELECT pg_advisory_unlock(hashtextextended(?,0))", key).Error != nil {
					if raw, ok := conn.Statement.ConnPool.(*sql.Conn); ok {
						_ = raw.Raw(func(any) error { return driver.ErrBadConn })
					}
				}
			}()
			for {
				var acquired bool
				if err := conn.Session(&gorm.Session{NewDB: true}).Raw("SELECT pg_try_advisory_lock(hashtextextended(?,0))", key).Scan(&acquired).Error; err != nil {
					return err
				}
				if acquired {
					return fn(conn.Session(&gorm.Session{NewDB: true}))
				}
				timer := time.NewTimer(20 * time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
		})
	}
	// Non-PostgreSQL adapters are process-local; no distributed guarantee is
	// advertised for these adapters. Share locks across service instances.
	db, err := r.db.DB()
	if err != nil {
		return err
	}
	key := fmt.Sprintf("%p:%s", db, id)
	fences.Lock()
	lock := fences.values[key]
	if lock == nil {
		lock = &localFence{token: make(chan struct{}, 1)}
		fences.values[key] = lock
	}
	lock.refs++
	fences.Unlock()
	defer func() {
		fences.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(fences.values, key)
		}
		fences.Unlock()
	}()
	select {
	case lock.token <- struct{}{}:
		defer func() { <-lock.token }()
		return fn(r.db.WithContext(ctx))
	case <-ctx.Done():
		return ctx.Err()
	}
}
