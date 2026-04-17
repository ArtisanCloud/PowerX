package backup_ops

import (
	"context"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repoops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"gorm.io/gorm"
)

type ArtifactCleanupService struct {
	policyRepo   *repoops.BackupPolicyRepository
	jobRepo      *repoops.BackupJobRepository
	artifactRepo *repoops.BackupArtifactRepository
}

type CleanupResult struct {
	DeletedArtifacts int `json:"deleted_artifacts"`
	DeletedJobs      int `json:"deleted_jobs"`
}

func NewArtifactCleanupService(db *gorm.DB) *ArtifactCleanupService {
	return &ArtifactCleanupService{
		policyRepo:   repoops.NewBackupPolicyRepository(db),
		jobRepo:      repoops.NewBackupJobRepository(db),
		artifactRepo: repoops.NewBackupArtifactRepository(db),
	}
}

// CleanupByPolicy 保留最近 N 份成功备份，仅清理备份产物，不删除任务记录。
func (s *ArtifactCleanupService) CleanupByPolicy(ctx context.Context, policyID uint64, retentionCount int) (*CleanupResult, error) {
	if policyID == 0 {
		return &CleanupResult{}, nil
	}
	if retentionCount <= 0 {
		retentionCount = 14
	}
	successJobs, err := s.jobRepo.ListByPolicyAndStatus(ctx, policyID, modelops.BackupJobStatusSuccess, 2000)
	if err != nil {
		return nil, err
	}
	if len(successJobs) <= retentionCount {
		return &CleanupResult{}, nil
	}

	keep := retentionCount
	if keep < 1 {
		keep = 1
	}

	result := &CleanupResult{}
	for idx := keep; idx < len(successJobs); idx++ {
		job := successJobs[idx]
		artifacts, listErr := s.artifactRepo.ListByJobID(ctx, job.ID)
		if listErr != nil {
			return nil, listErr
		}
		if delErr := s.artifactRepo.DeleteByJobID(ctx, job.ID); delErr != nil {
			return nil, delErr
		}
		result.DeletedArtifacts += len(artifacts)
	}
	return result, nil
}

func (s *ArtifactCleanupService) CleanupAllPolicies(ctx context.Context) (*CleanupResult, error) {
	items, _, err := s.policyRepo.ListWithFilters(ctx, "", "", "", nil, 2000, 0)
	if err != nil {
		return nil, err
	}
	merged := &CleanupResult{}
	for i := range items {
		policy := items[i]
		retention := int(policy.RetentionCount)
		if retention <= 0 {
			retention = int(policy.RetentionDays)
		}
		if retention <= 0 {
			retention = 14
		}
		part, cleanupErr := s.CleanupByPolicy(ctx, policy.ID, retention)
		if cleanupErr != nil {
			return nil, cleanupErr
		}
		merged.DeletedArtifacts += part.DeletedArtifacts
		merged.DeletedJobs += part.DeletedJobs
	}
	return merged, nil
}
