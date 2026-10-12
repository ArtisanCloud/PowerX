package backup_ops

import (
	"context"
	"errors"
	"fmt"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repoops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"gorm.io/gorm"
	"os"
	"time"
)

type ArtifactCleanupService struct {
	db           *gorm.DB
	policyRepo   *repoops.BackupPolicyRepository
	jobRepo      *repoops.BackupJobRepository
	artifactRepo *repoops.BackupArtifactRepository
}
type CleanupResult struct {
	DeletedArtifacts int `json:"deleted_artifacts"`
	DeletedJobs      int `json:"deleted_jobs"`
}

func NewArtifactCleanupService(db *gorm.DB) *ArtifactCleanupService {
	return &ArtifactCleanupService{db: db, policyRepo: repoops.NewBackupPolicyRepository(db), jobRepo: repoops.NewBackupJobRepository(db), artifactRepo: repoops.NewBackupArtifactRepository(db)}
}

// CleanupByPolicy 读取持久化保留合同；任务记录保留，过期文件删除后再软删产物记录。
func (s *ArtifactCleanupService) CleanupByPolicy(ctx context.Context, policyID uint64) (*CleanupResult, error) {
	if policyID == 0 {
		return &CleanupResult{}, nil
	}
	unlock, err := lockBackupPolicy(ctx, s.db, policyID)
	if err != nil {
		return nil, err
	}
	defer unlock()
	policy, err := s.policyRepo.GetById(ctx, policyID, nil)
	if err != nil {
		return nil, err
	}
	if policy == nil {
		return nil, ErrBackupPolicyNotFound
	}
	return s.cleanupPolicy(ctx, policy)
}
func (s *ArtifactCleanupService) cleanupPolicy(ctx context.Context, policy *modelops.BackupPolicy) (*CleanupResult, error) {
	root, err := resolveBackupArtifactBaseDir()
	if err != nil {
		return nil, err
	}
	jobs, err := s.jobRepo.ListByPolicyAndStatus(ctx, policy.ID, modelops.BackupJobStatusSuccess, 0)
	if err != nil {
		return nil, err
	}
	result := &CleanupResult{}
	index := 0
	for _, job := range jobs {
		artifacts, e := s.artifactRepo.ListByJobID(ctx, job.ID)
		if e != nil {
			return result, e
		}
		if len(artifacts) == 0 {
			continue
		}
		expired := backupExpired(job, index, *policy, time.Now().UTC())
		index++
		if !expired {
			continue
		}
		for i := range artifacts {
			artifact := artifacts[i]
			path, e := validatedArtifactPath(root, &artifact)
			if e != nil && !errors.Is(e, os.ErrNotExist) {
				return result, fmt.Errorf("artifact %d: %w", artifact.ID, e)
			}
			if e == nil {
				if e = verifyArtifactFile(path, &artifact); e != nil {
					return result, e
				}
			}
			if e = removeArtifactFiles(path, &artifact); e != nil {
				return result, e
			}
			if e = s.artifactRepo.DeleteByID(ctx, artifact.ID); e != nil {
				return result, e
			}
			result.DeletedArtifacts++
		}
	}
	return result, nil
}
func (s *ArtifactCleanupService) CleanupAllPolicies(ctx context.Context) (*CleanupResult, error) {
	items, _, err := s.policyRepo.ListWithFilters(ctx, "", "", "", nil, 0, 0)
	if err != nil {
		return nil, err
	}
	merged := &CleanupResult{}
	for _, p := range items {
		part, e := s.CleanupByPolicy(ctx, p.ID)
		if e != nil {
			return merged, e
		}
		merged.DeletedArtifacts += part.DeletedArtifacts
	}
	return merged, nil
}
