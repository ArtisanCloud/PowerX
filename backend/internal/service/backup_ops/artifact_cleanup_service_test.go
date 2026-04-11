package backup_ops

import (
	"context"
	"testing"
)

func TestArtifactCleanupService_CleanupByPolicy_IdempotentWhenPolicyZero(t *testing.T) {
	svc := &ArtifactCleanupService{}
	r1, err := svc.CleanupByPolicy(context.Background(), 0, 14)
	if err != nil {
		t.Fatalf("first cleanup should not error: %v", err)
	}
	r2, err := svc.CleanupByPolicy(context.Background(), 0, 14)
	if err != nil {
		t.Fatalf("second cleanup should not error: %v", err)
	}
	if r1.DeletedArtifacts != 0 || r1.DeletedJobs != 0 || r2.DeletedArtifacts != 0 || r2.DeletedJobs != 0 {
		t.Fatalf("cleanup with policy=0 should be no-op: r1=%+v r2=%+v", r1, r2)
	}
}
