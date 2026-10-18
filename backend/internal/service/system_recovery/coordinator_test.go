package system_recovery

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
)

type testDriver struct {
	candidateID string
	active      string
	held        bool
	stopped     []string
	captures    int
	rollbackID  string
	failStart   bool
	failVerify  bool
	newWrites   int
}

func (d *testDriver) Capture(ctx context.Context, j *snapshots.ManagedJob, deployment Deployment) error {
	d.captures++
	if j.Kind == "cutover" || j.Kind == "overwrite" || j.Kind == "rollback" || j.Kind == "restore" {
		j.RollbackSnapshotID = d.rollbackID
	}
	return nil
}
func (d *testDriver) Prepare(context.Context, *snapshots.ManagedJob) (Deployment, []snapshots.RecoveryCheck, error) {
	return Deployment{ID: d.candidateID}, []snapshots.RecoveryCheck{{Name: "business", Passed: true}}, nil
}
func (d *testDriver) Stop(ctx context.Context, deployment Deployment) error {
	d.stopped = append(d.stopped, deployment.ID)
	d.active = ""
	return nil
}
func (d *testDriver) Start(ctx context.Context, deployment Deployment, hold bool) error {
	if d.failStart && deployment.ID == d.candidateID {
		return errors.New("candidate failed")
	}
	d.active = deployment.ID
	d.held = hold
	if !hold {
		d.newWrites++
	}
	return nil
}
func (d *testDriver) Verify(ctx context.Context, deployment Deployment) ([]snapshots.RecoveryCheck, error) {
	if d.failVerify && deployment.ID == d.candidateID {
		return nil, errors.New("business check failed")
	}
	return []snapshots.RecoveryCheck{{Name: "business", Passed: true}}, nil
}
func coordinatorFixture(t *testing.T) (*Coordinator, *testDriver, snapshots.ManagedJob) {
	t.Helper()
	root := t.TempDir()
	store, e := snapshots.NewCoordinatorStore(root, []snapshots.RecoveryTarget{{ID: "formal", Mode: "formal", Environment: "test"}})
	if e != nil {
		t.Fatal(e)
	}
	driver := &testDriver{active: "source", rollbackID: uuid.NewString()}
	c := &Coordinator{Store: store, Driver: driver, Initial: Deployment{ID: "source"}}
	if _, e = c.Active(); e != nil {
		t.Fatal(e)
	}
	candidateID := uuid.NewString()
	driver.candidateID = candidateID
	candidate := snapshots.ManagedJob{ID: candidateID, Kind: "prepare", TargetID: "formal", Status: "verified", BusinessVerified: true, SnapshotID: uuid.NewString()}
	if e = store.Save(candidate); e != nil {
		t.Fatal(e)
	}
	if e = snapshots.WriteCoordinatorJSON(c.candidateFile(candidateID), Deployment{ID: candidateID}); e != nil {
		t.Fatal(e)
	}
	job := snapshots.ManagedJob{ID: uuid.NewString(), Kind: "cutover", Status: "queued", CandidateID: candidateID, SnapshotID: candidate.SnapshotID, TargetID: "formal", ExpectedGeneration: 1}
	if e = store.Save(job); e != nil {
		t.Fatal(e)
	}
	return c, driver, job
}

func TestRecoveryWholeSwitch(t *testing.T) {
	c, d, j := coordinatorFixture(t)
	if e := c.RunOnce(context.Background()); e != nil {
		t.Fatal(e)
	}
	j, e := c.Store.Get(j.ID)
	if e != nil {
		t.Fatal(e)
	}
	active, e := c.Active()
	if e != nil {
		t.Fatal(e)
	}
	if j.Status != "complete" || !j.WritesMayHaveResumed || j.RollbackSnapshotID != d.rollbackID || active.Deployment.ID != j.CandidateID || active.Maintenance || active.Generation != 2 || d.active != j.CandidateID || d.held || d.captures != 1 {
		t.Fatalf("incomplete switch: job=%+v active=%+v driver=%+v", j, active, d)
	}
}
func TestRecoveryFailuresCompensateBeforeWrites(t *testing.T) {
	for _, phase := range []string{"maintenance", "writers_stopped", "rollback_point_saved", "pointer_switched", "candidate_started_read_only", "candidate_verified"} {
		t.Run(phase, func(t *testing.T) {
			c, d, j := coordinatorFixture(t)
			c.Checkpoint = func(p string) error {
				if p == phase {
					return errors.New("injected failure")
				}
				return nil
			}
			if e := c.RunOnce(context.Background()); e != nil {
				t.Fatal(e)
			}
			j, _ = c.Store.Get(j.ID)
			a, _ := c.Active()
			if j.Status != "rolled_back" || j.WritesMayHaveResumed || a.Deployment.ID != "source" || a.Maintenance || d.active != "source" || d.held {
				t.Fatalf("not fully compensated: %+v %+v %+v", j, a, d)
			}
		})
	}
}
func TestRecoveryInterruptionCompensatesFromPersistedJournal(t *testing.T) {
	for _, phase := range []string{"maintenance", "writers_stopped", "rollback_point_saved", "pointer_switched", "candidate_started_read_only", "candidate_verified"} {
		t.Run(phase, func(t *testing.T) {
			c, d, j := coordinatorFixture(t)
			c.Checkpoint = func(p string) error {
				if p == phase {
					panic("crash")
				}
				return nil
			}
			func() { defer func() { _ = recover() }(); _ = c.RunOnce(context.Background()) }()
			c.Checkpoint = nil
			if e := c.Recover(context.Background()); e != nil {
				t.Fatal(e)
			}
			j, _ = c.Store.Get(j.ID)
			a, _ := c.Active()
			if j.Status != "rolled_back" || a.Maintenance || a.Deployment.ID != "source" || d.active != "source" {
				t.Fatalf("crash recovery failed: %+v %+v", j, a)
			}
		})
	}
}
func TestRecoveryNoAutomaticRollbackAfterWritesMayResume(t *testing.T) {
	c, d, j := coordinatorFixture(t)
	c.Checkpoint = func(phase string) error {
		if phase == "resuming_writes" {
			return errors.New("process interrupted")
		}
		return nil
	}
	if e := c.RunOnce(context.Background()); e != nil {
		t.Fatal(e)
	}
	j, _ = c.Store.Get(j.ID)
	a, _ := c.Active()
	if j.Status != "attention_required" || !j.WritesMayHaveResumed || a.Deployment.ID != j.CandidateID || !a.Maintenance || d.active == "source" {
		t.Fatalf("unsafe automatic rollback: %+v %+v", j, a)
	}
}

func TestRecoveryResumeRetainsCandidateData(t *testing.T) {
	c, d, job := coordinatorFixture(t)
	c.Checkpoint = func(phase string) error {
		if phase == "resuming_writes" {
			return errors.New("interrupted")
		}
		return nil
	}
	if e := c.RunOnce(context.Background()); e != nil {
		t.Fatal(e)
	}
	job, _ = c.Store.Get(job.ID)
	if job.Status != "attention_required" || !d.held || d.active != job.CandidateID {
		t.Fatal("candidate was not held")
	}
	c.Checkpoint = nil
	resume := snapshots.ManagedJob{ID: uuid.NewString(), Kind: "resume", Status: "queued", SnapshotID: job.SnapshotID, TargetID: job.TargetID, CandidateID: job.CandidateID, ExpectedGeneration: 2}
	if e := c.Store.Save(resume); e != nil {
		t.Fatal(e)
	}
	if e := c.RunOnce(context.Background()); e != nil {
		t.Fatal(e)
	}
	resumed, _ := c.Store.Get(resume.ID)
	previous, _ := c.Store.Get(job.ID)
	active, _ := c.Active()
	if resumed.Status != "complete" || previous.Status != "resolved" || active.Maintenance || active.Deployment.ID != job.CandidateID || d.held || d.active != job.CandidateID {
		t.Fatalf("unsafe resume: %+v %+v %+v", resumed, previous, active)
	}
}
func TestRecoveryGenerationRejectsStaleConfirmation(t *testing.T) {
	c, d, j := coordinatorFixture(t)
	j.ExpectedGeneration = 9
	if e := c.Store.Save(j); e != nil {
		t.Fatal(e)
	}
	if e := c.RunOnce(context.Background()); e != nil {
		t.Fatal(e)
	}
	j, _ = c.Store.Get(j.ID)
	a, _ := c.Active()
	if j.Status != "failed" || len(d.stopped) != 0 || a.Generation != 1 || a.Deployment.ID != "source" {
		t.Fatal("stale confirmation changed source")
	}
}
func TestRecoveryInterruptedPrepareDoesNotTouchSource(t *testing.T) {
	c, d, j := coordinatorFixture(t)
	j.Kind = "prepare"
	j.Status = "running"
	if e := c.Store.Save(j); e != nil {
		t.Fatal(e)
	}
	if e := c.Recover(context.Background()); e != nil {
		t.Fatal(e)
	}
	j, _ = c.Store.Get(j.ID)
	if j.Status != "interrupted" || len(d.stopped) != 0 || d.active != "source" {
		t.Fatal("prepare recovery changed source")
	}
	if _, e := filepath.Abs(c.Store.Directory); e != nil {
		t.Fatal(e)
	}
}

func (d *testDriver) CaptureOnline(ctx context.Context, j *snapshots.ManagedJob, deployment Deployment) error {
	if len(d.stopped) > 0 || d.active != deployment.ID {
		return errors.New("online capture stopped business")
	}
	d.captures++
	return nil
}

type noOnlineDriver struct{ Driver }

func TestScheduledCaptureNeverStopsSource(t *testing.T) {
	for _, supported := range []bool{true, false} {
		c, d, _ := coordinatorFixture(t)
		if !supported {
			c.Driver = noOnlineDriver{Driver: d}
		}
		j := snapshots.ManagedJob{ID: uuid.NewString(), Kind: "capture", Status: "running", SnapshotID: uuid.NewString()}
		err := c.capture(context.Background(), &j)
		if supported && err != nil || !supported && err == nil {
			t.Fatalf("supported=%v err=%v", supported, err)
		}
		a, e := c.Active()
		if e != nil {
			t.Fatal(e)
		}
		if len(d.stopped) != 0 || a.Maintenance || d.active != "source" {
			t.Fatalf("business interrupted: %+v %+v", a, d)
		}
	}
}

func TestInterruptedOnlineCaptureDoesNotRestartBusiness(t *testing.T) {
	c, d, j := coordinatorFixture(t)
	j.Kind = "capture"
	j.Status = "running"
	j.Phase = "capturing_online"
	if e := c.Store.Save(j); e != nil {
		t.Fatal(e)
	}
	if e := c.Recover(context.Background()); e != nil {
		t.Fatal(e)
	}
	got, e := c.Store.Get(j.ID)
	if e != nil {
		t.Fatal(e)
	}
	active, e := c.Active()
	if e != nil {
		t.Fatal(e)
	}
	if got.Status != "interrupted" || len(d.stopped) > 0 || d.newWrites > 0 || active.Maintenance || d.active != "source" {
		t.Fatalf("online recovery changed business: %+v %+v %+v", got, active, d)
	}
}
