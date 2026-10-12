package backup_ops

import (
	"testing"
)

func TestJobService_TryLockPolicy_ReentrantBlocked(t *testing.T) {
	svc := &JobService{
		policyLock: make(map[uint64]struct{}),
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
