package backup_ops

import (
	"context"
	"errors"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"testing"
)

func TestOneTimeDatabaseExportRequiresRootUserBeforeDatabaseAccess(t *testing.T) {
	sts := &reqctx.CoreXClaims{IsRoot: true}
	sts.Issuer = "powerx-sts"
	for _, ctx := range []context.Context{
		context.Background(),
		reqctx.WithClaims(reqctx.WithIsRoot(context.Background(), true), sts),
		reqctx.WithClaims(reqctx.WithIsRoot(context.Background(), true), &reqctx.CoreXClaims{IsRoot: true, Platforms: []string{"api_key"}}),
	} {
		_, err := (&JobService{}).TriggerJob(ctx, TriggerJobRequest{OneTime: true})
		if dto.StatusCode(ToAppError(err)) != 403 {
			t.Fatalf("non-user export reached database: %v", err)
		}
		_, _, err = (&JobService{}).OpenDownload(ctx, 1, "test")
		if dto.StatusCode(ToAppError(err)) != 403 {
			t.Fatalf("non-user download reached database: %v", err)
		}
	}
}

func TestOneTimeBackupRejectsAmbiguousOrScheduledRequests(t *testing.T) {
	for _, request := range []TriggerJobRequest{{}, {OneTime: true, PolicyID: 1}, {OneTime: true, TriggerType: modelops.BackupTriggerTypeScheduled}} {
		_, err := (&JobService{}).TriggerJob(context.Background(), request)
		if !errors.Is(err, ErrInvalidBackupRequest) {
			t.Fatalf("invalid request reached database: %+v %v", request, err)
		}
	}
}

func TestJobServiceRejectsUnknownListViewBeforeQuery(t *testing.T) {
	_, _, err := (&JobService{}).ListJobs(context.Background(), ListJobOptions{View: "delete"})
	if !errors.Is(err, ErrInvalidBackupRequest) {
		t.Fatalf("unknown view must fail without querying: %v", err)
	}
}

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
