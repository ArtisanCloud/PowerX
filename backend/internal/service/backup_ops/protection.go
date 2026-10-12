package backup_ops

import (
	"context"
	"fmt"
	obsops "github.com/ArtisanCloud/PowerX/internal/service/observability_ops"
)

func (s *JobService) SetProtected(ctx context.Context, jobID uint64, protected bool, operator, traceID string) error {
	job, err := s.GetJob(ctx, jobID)
	if err != nil {
		return err
	}
	unlock, err := lockBackupPolicy(ctx, s.db, job.PolicyID)
	if err != nil {
		return err
	}
	defer unlock()
	artifact, err := s.artifactRepo.GetLatestByJobID(ctx, jobID)
	if err != nil {
		return err
	}
	if artifact == nil {
		return ErrInvalidBackupRequest
	}
	path, err := validatedArtifactPath(s.artifactBaseDir, artifact)
	if err != nil {
		return err
	}
	if err = verifyArtifactFile(path, artifact); err != nil {
		return err
	}
	if err = s.jobRepo.SetProtected(ctx, jobID, protected); err != nil {
		return err
	}
	s.audit(ctx, obsops.AuditRecord{ResourceType: "backup_job", ResourceID: fmt.Sprint(jobID), Operation: "set_protected", Outcome: "success", Severity: "info", Detail: map[string]any{"protected": protected, "operator": normalizeOperator(operator), "trace_id": traceID}})
	return nil
}
