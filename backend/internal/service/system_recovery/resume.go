package system_recovery

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
)

func (c *Coordinator) resume(ctx context.Context, j *snapshots.ManagedJob) (result error) {
	active, e := c.Active()
	if e != nil {
		return e
	}
	if !active.Maintenance || active.Generation != j.ExpectedGeneration || active.Deployment.ID != j.CandidateID {
		return errors.New("维护现场或部署代次已经改变")
	}
	var candidate Deployment
	if active.Deployment.ID == c.Initial.ID {
		candidate = active.Deployment
	} else if e = snapshots.ReadCoordinatorJSON(c.candidateFile(j.CandidateID), &candidate); e != nil {
		return e
	}
	defer func() {
		if result != nil {
			j.Status = "attention_required"
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer cancel()
			if e := c.holdCandidate(cleanup, j, candidate); e != nil {
				result = fmt.Errorf("%w; hold failed: %v", result, e)
			}
		}
	}()
	if e = c.Driver.Start(ctx, candidate, true); e != nil {
		return e
	}
	checks, e := c.Driver.Verify(ctx, candidate)
	j.Checks = checks
	if e != nil {
		return e
	}
	if len(checks) == 0 {
		return errors.New("empty verification report")
	}
	for _, check := range checks {
		if !check.Passed {
			return errors.New("candidate verification failed")
		}
	}
	if e = c.Driver.Stop(ctx, candidate); e != nil {
		return e
	}
	j.WritesMayHaveResumed = true
	if e = c.step(j, "resuming_writes"); e != nil {
		return e
	}
	if e = c.Driver.Start(ctx, candidate, false); e != nil {
		return e
	}
	checks, e = c.Driver.Verify(ctx, candidate)
	j.Checks = checks
	if e != nil {
		return e
	}
	if len(checks) == 0 {
		return errors.New("empty active verification")
	}
	for _, check := range checks {
		if !check.Passed {
			return errors.New("active verification failed")
		}
	}
	active.Maintenance = false
	active.JobID = ""
	if e = c.saveActive(active); e != nil {
		return e
	}
	jobs, e := c.Store.List()
	if e != nil {
		return e
	}
	for _, previous := range jobs {
		if previous.ID != j.ID && previous.CandidateID == j.CandidateID && previous.Status == "attention_required" {
			previous.Status = "resolved"
			previous.Phase = "resolved_by:" + j.ID
			if e = c.Store.Save(previous); e != nil {
				return e
			}
		}
	}
	j.Status = "complete"
	j.Phase = "complete"
	j.BusinessVerified = true
	return nil
}

// QueueResumeCandidate 由本机运维账户明确确认，仅写持久队列，不绕过协调器执行锁。
func QueueResumeCandidate(ctx context.Context, file, jobID, confirmation string) (snapshots.ManagedJob, error) {
	if confirmation != jobID {
		return snapshots.ManagedJob{}, errors.New("-confirmation must exactly match the reviewed job UUID")
	}
	profile, e := ReadProfile(file)
	if e != nil {
		return snapshots.ManagedJob{}, e
	}
	store, e := snapshots.NewCoordinatorStore(profile.Directory, profile.Targets)
	if e != nil {
		return snapshots.ManagedJob{}, e
	}
	var active Active
	if e = snapshots.ReadCoordinatorJSON(filepath.Join(profile.Directory, "active.json"), &active); e != nil {
		return snapshots.ManagedJob{}, e
	}
	return store.EnqueueResume(privileged(ctx), jobID, uuid.NewString(), active.Generation)
}
