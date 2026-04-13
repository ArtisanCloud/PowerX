package backup_ops

import (
	"testing"
	"time"
)

func TestJobService_TryLockPolicy_ReentrantBlocked(t *testing.T) {
	svc := &JobService{
		policyLock: make(map[uint64]struct{}),
		nextRuns:   make(map[uint64]time.Time),
	}
	if ok := svc.tryLockPolicy(7); !ok {
		t.Fatalf("first lock should succeed")
	}
	if ok := svc.tryLockPolicy(7); ok {
		t.Fatalf("second lock should be blocked")
	}
	svc.unlockPolicy(7)
	if ok := svc.tryLockPolicy(7); !ok {
		t.Fatalf("lock after unlock should succeed")
	}
}

func TestJobService_NextRunAt(t *testing.T) {
	svc := &JobService{
		policyLock: make(map[uint64]struct{}),
		nextRuns:   make(map[uint64]time.Time),
	}
	now := time.Date(2026, 4, 11, 10, 0, 0, 0, time.UTC)
	next := svc.nextRunAt(1, now, "6h")
	if !next.Equal(now.Add(6 * time.Hour)) {
		t.Fatalf("next run mismatch, got %s", next)
	}

	byMinute := svc.nextRunAt(2, now, "30m")
	if !byMinute.Equal(now.Add(30 * time.Minute)) {
		t.Fatalf("minute next run mismatch, got %s", byMinute)
	}

	byDay := svc.nextRunAt(3, now, "2d")
	if !byDay.Equal(now.Add(48 * time.Hour)) {
		t.Fatalf("day next run mismatch, got %s", byDay)
	}

	fallback := svc.nextRunAt(4, now, "bad")
	if !fallback.Equal(now.Add(time.Duration(defaultIntervalHours) * time.Hour)) {
		t.Fatalf("fallback next run mismatch, got %s", fallback)
	}
}
