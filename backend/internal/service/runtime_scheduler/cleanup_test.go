package runtimescheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/runtime_scheduler"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

func cleanupRootService(t *testing.T) (*Service, context.Context) {
	t.Helper()
	return newTestService(t, func(c *reqctx.CoreXClaims) { c.IsRoot = true })
}
func cleanupFixture(t *testing.T, s *Service, ctx context.Context) *models.SchedulerJob {
	t.Helper()
	job, err := s.CreateJob(ctx, JobSpec{OwnerType: models.OwnerTypePlugin, OwnerID: "com.powerx.plugins.ai-craft", Name: "cleanup-fixture", ScheduleType: models.ScheduleTypeInterval, ScheduleExpr: "15m", Payload: map[string]any{"purpose": "core-test"}}, "test-root", "create-trace")
	require.NoError(t, err)
	return job
}
func TestCleanupRevisionHistoryIdempotencyAndNameReuse(t *testing.T) {
	s, ctx := cleanupRootService(t)
	job := cleanupFixture(t, s, ctx)
	require.EqualValues(t, 1, job.Revision)
	_, err := s.DeleteJob(ctx, DeleteJobInput{JobUUID: job.UUID.String()})
	require.Equal(t, "SCHEDULER_EXPECTED_REVISION_REQUIRED", dto.CodeOf(err))
	paused, err := s.PauseJob(ctx, job.UUID.String(), "root", "pause-trace")
	require.NoError(t, err)
	require.EqualValues(t, 2, paused.Revision)
	_, err = s.DeleteJob(ctx, DeleteJobInput{JobUUID: job.UUID.String(), ExpectedRevision: 1})
	require.Equal(t, 409, dto.StatusCode(err))
	_, err = s.ResumeJob(ctx, job.UUID.String(), "root", "resume-trace")
	require.NoError(t, err)
	triggered, err := s.TriggerJob(ctx, job.UUID.String(), "root", "trigger-trace")
	require.NoError(t, err)
	deleted, err := s.DeleteJob(reqctx.WithTraceID(ctx, "delete-trace"), DeleteJobInput{JobUUID: job.UUID.String(), ExpectedRevision: triggered.Job.Revision})
	require.NoError(t, err)
	require.False(t, deleted.AlreadyDeleted)
	require.True(t, deleted.HistoryRetained)
	repeated, err := s.DeleteJob(ctx, DeleteJobInput{JobUUID: job.UUID.String(), ExpectedRevision: 1})
	require.NoError(t, err)
	require.True(t, repeated.AlreadyDeleted)
	require.Equal(t, deleted.Revision, repeated.Revision)
	jobs, _, err := s.ListJobs(ctx, ListJobsInput{})
	require.NoError(t, err)
	require.Empty(t, jobs)
	due, err := s.jobs.ListDue(ctx, time.Now().Add(24*time.Hour), 100)
	require.NoError(t, err)
	require.Empty(t, due)
	runs, total, err := s.ListCleanupRuns(ctx, ListRunsInput{JobID: job.UUID.String()})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, runs, 1)
	_, err = s.TriggerJob(ctx, job.UUID.String(), "root", "late")
	require.Error(t, err)
	_, err = s.ResumeJob(ctx, job.UUID.String(), "root", "late")
	require.Error(t, err)
	_, err = s.UpdateJob(ctx, UpdateJobInput{JobID: job.UUID.String()})
	require.Error(t, err)
	recreated := cleanupFixture(t, s, ctx)
	require.NotEqual(t, job.UUID, recreated.UUID)
	tombstone, err := s.GetCleanupJob(ctx, job.UUID.String(), true)
	require.NoError(t, err)
	require.NotNil(t, tombstone.DeletedAt)
}
func TestCleanupWaitsForIssuedPublicationAndStopsFutureTriggers(t *testing.T) {
	s, ctx := cleanupRootService(t)
	job := cleanupFixture(t, s, ctx)
	entered, release := make(chan struct{}), make(chan struct{})
	var publications atomic.Int64
	s.eventBu = eventBusFunc(func(_ string, _ any, _ context.Context) { publications.Add(1); close(entered); <-release })
	triggerDone := make(chan error, 1)
	go func() { _, err := s.TriggerJob(ctx, job.UUID.String(), "root", "trigger"); triggerDone <- err }()
	<-entered
	deletionDone := make(chan error, 1)
	go func() {
		_, err := s.DeleteJob(ctx, DeleteJobInput{JobUUID: job.UUID.String(), ExpectedRevision: 2})
		deletionDone <- err
	}()
	select {
	case err := <-deletionDone:
		t.Fatalf("deletion returned before publication finished: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-triggerDone)
	require.NoError(t, <-deletionDone)
	_, err := s.TriggerJob(ctx, job.UUID.String(), "root", "after-delete")
	require.Error(t, err)
	require.EqualValues(t, 1, publications.Load())
}
func TestCleanupCrossTenantAndAdminPermission(t *testing.T) {
	s, ctx := cleanupRootService(t)
	job := cleanupFixture(t, s, ctx)
	foreign := reqctx.WithTenantUUID(ctx, "1edd4132-1644-412d-abb4-d5f1e9487052")
	_, err := s.DeleteJob(foreign, DeleteJobInput{JobUUID: job.UUID.String(), ExpectedRevision: 1})
	require.Equal(t, 404, dto.StatusCode(err))
	_, err = NewCleanupInvoker(s).InvokeCoreCapability(ctx, capInput(CleanupDeleteCapabilityID, map[string]any{"operation": "delete_job", "job_uuid": job.UUID.String(), "expected_revision": 1}))
	require.Equal(t, 403, dto.StatusCode(err))
}

func capInput(id string, body map[string]any) cap.CoreCapabilityInvokeInput {
	return cap.CoreCapabilityInvokeInput{CapabilityID: id, TenantUUID: schedulerTestTenant, Method: "INVOKE", Endpoint: "core://scheduler/jobs", Body: body}
}

func TestCleanupAutomaticDispatcherUsesPublicationFence(t *testing.T) {
	s, ctx := cleanupRootService(t)
	job := cleanupFixture(t, s, ctx)
	entered, release := make(chan struct{}), make(chan struct{})
	s.eventBu = eventBusFunc(func(_ string, _ any, _ context.Context) { close(entered); <-release })
	fired := make(chan error, 1)
	go func() { _, err := s.dispatchDueJob(ctx, job.UUID, s.clock().Add(16*time.Minute)); fired <- err }()
	<-entered
	deleted := make(chan error, 1)
	go func() {
		_, err := s.DeleteJob(ctx, DeleteJobInput{JobUUID: job.UUID.String(), ExpectedRevision: 2})
		deleted <- err
	}()
	select {
	case err := <-deleted:
		t.Fatalf("delete returned during automatic publication: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-fired)
	require.NoError(t, <-deleted)
	dispatched, err := s.dispatchDueJob(ctx, job.UUID, s.clock().Add(48*time.Hour))
	require.NoError(t, err)
	require.False(t, dispatched)
}
