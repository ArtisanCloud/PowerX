package skills

import (
	"context"
	"errors"
	"strings"

	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
)

// LifecycleService controls publish/rollback lifecycle state transitions.
type LifecycleService struct {
	registryRepo *skillrepo.SkillRegistryRepository
	auditService *AuditTraceService
}

func NewLifecycleService(
	registryRepo *skillrepo.SkillRegistryRepository,
	auditService *AuditTraceService,
) *LifecycleService {
	if registryRepo == nil {
		panic("lifecycle service requires registry repository")
	}
	return &LifecycleService{
		registryRepo: registryRepo,
		auditService: auditService,
	}
}

func (s *LifecycleService) Publish(ctx context.Context, skillID, version, operator, approvalNote string) error {
	skillID = strings.TrimSpace(skillID)
	version = strings.TrimSpace(version)
	if skillID == "" || version == "" {
		return errors.New("skill_id and version are required")
	}

	rec, err := s.registryRepo.GetBySkillVersion(ctx, skillID, version)
	if err != nil {
		return err
	}
	if rec.Status == skillmodel.SkillStatusDisabled {
		return errors.New("disabled skill version cannot be published")
	}
	if rec.Checksum == "" {
		return errors.New("checksum is required before publish")
	}

	if err := s.registryRepo.SetLatestPublished(ctx, skillID, version, operator, approvalNote); err != nil {
		return err
	}

	if s.auditService != nil {
		_ = s.auditService.RecordLifecycleAudit(ctx, LifecycleAuditInput{
			Action:   "publish",
			SkillID:  skillID,
			Version:  version,
			Operator: operator,
			Source:   rec.Source,
			Result:   "success",
			Reason:   approvalNote,
		})
	}
	return nil
}

func (s *LifecycleService) Rollback(ctx context.Context, skillID, targetVersion, operator, reason string) error {
	skillID = strings.TrimSpace(skillID)
	targetVersion = strings.TrimSpace(targetVersion)
	if skillID == "" || targetVersion == "" {
		return errors.New("skill_id and target_version are required")
	}

	rec, err := s.registryRepo.GetBySkillVersion(ctx, skillID, targetVersion)
	if err != nil {
		return err
	}
	if rec.Status != skillmodel.SkillStatusPublished && rec.Status != skillmodel.SkillStatusDeprecated {
		return errors.New("target version must be published or deprecated")
	}

	if err := s.registryRepo.SetLatestPublished(ctx, skillID, targetVersion, operator, reason); err != nil {
		return err
	}

	if s.auditService != nil {
		_ = s.auditService.RecordLifecycleAudit(ctx, LifecycleAuditInput{
			Action:   "rollback",
			SkillID:  skillID,
			Version:  targetVersion,
			Operator: operator,
			Source:   rec.Source,
			Result:   "success",
			Reason:   reason,
		})
	}
	return nil
}
