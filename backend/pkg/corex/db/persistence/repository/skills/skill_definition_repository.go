package skills

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
)

var (
	ErrSkillDefinitionDraftNotFound    = errors.New("skill.definition_draft_not_found")
	ErrSkillDefinitionRevisionNotFound = errors.New("skill.definition_revision_not_found")
	ErrSkillPackageSourceNotFound      = errors.New("skill.package_source_not_found")
)

// SkillDefinitionRepository owns the tenant-scoped definition lifecycle.
// Registry records are deliberately not used here: a draft/revision is the
// source of truth until an explicit publish creates its immutable artifact.
type SkillDefinitionRepository struct {
	db *gorm.DB
}

func NewSkillDefinitionRepository(db *gorm.DB) *SkillDefinitionRepository {
	if db == nil {
		panic("skill definition repository requires db")
	}
	return &SkillDefinitionRepository{db: db}
}

func (r *SkillDefinitionRepository) CreatePackageSource(ctx context.Context, row *models.SkillPackageSource) (*models.SkillPackageSource, error) {
	if row == nil {
		return nil, gorm.ErrInvalidData
	}
	row.Normalize()
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func (r *SkillDefinitionRepository) GetPackageSource(ctx context.Context, tenantUUID, packageSourceUUID string) (*models.SkillPackageSource, error) {
	var row models.SkillPackageSource
	err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND uuid = ?", normalizeUUID(tenantUUID), normalizeUUID(packageSourceUUID)).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSkillPackageSourceNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *SkillDefinitionRepository) CreateDraftWithInitialRevision(
	ctx context.Context,
	draft *models.SkillDefinitionDraft,
	revision *models.SkillDefinitionRevision,
) (*models.SkillDefinitionDraft, *models.SkillDefinitionRevision, error) {
	if draft == nil || revision == nil {
		return nil, nil, gorm.ErrInvalidData
	}
	draft.Normalize()
	revision.Normalize()
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(draft).Error; err != nil {
			return err
		}
		revision.TenantUUID = draft.TenantUUID
		revision.DraftUUID = draft.UUID.String()
		revision.RevisionNumber = 1
		if err := tx.Create(revision).Error; err != nil {
			return err
		}
		return tx.Model(&models.SkillDefinitionDraft{}).
			Where("tenant_uuid = ? AND uuid = ?", draft.TenantUUID, draft.UUID.String()).
			Updates(map[string]any{
				"current_revision_uuid":  revision.UUID.String(),
				"updated_by_member_uuid": draft.UpdatedByMemberUUID,
			}).Error
	}); err != nil {
		return nil, nil, err
	}
	draft.CurrentRevisionUUID = revision.UUID.String()
	return draft, revision, nil
}

func (r *SkillDefinitionRepository) GetDraft(ctx context.Context, tenantUUID, draftUUID string) (*models.SkillDefinitionDraft, error) {
	var row models.SkillDefinitionDraft
	err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND uuid = ?", normalizeUUID(tenantUUID), normalizeUUID(draftUUID)).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSkillDefinitionDraftNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *SkillDefinitionRepository) GetDraftBySkillID(ctx context.Context, tenantUUID, skillID string) (*models.SkillDefinitionDraft, error) {
	var row models.SkillDefinitionDraft
	err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND skill_id = ?", normalizeUUID(tenantUUID), strings.ToLower(strings.TrimSpace(skillID))).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSkillDefinitionDraftNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *SkillDefinitionRepository) GetRevision(ctx context.Context, tenantUUID, revisionUUID string) (*models.SkillDefinitionRevision, error) {
	var row models.SkillDefinitionRevision
	err := r.db.WithContext(ctx).
		Where("tenant_uuid = ? AND uuid = ?", normalizeUUID(tenantUUID), normalizeUUID(revisionUUID)).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSkillDefinitionRevisionNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *SkillDefinitionRepository) GetCurrentRevision(ctx context.Context, tenantUUID, draftUUID string) (*models.SkillDefinitionRevision, error) {
	draft, err := r.GetDraft(ctx, tenantUUID, draftUUID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(draft.CurrentRevisionUUID) == "" {
		return nil, ErrSkillDefinitionRevisionNotFound
	}
	return r.GetRevision(ctx, tenantUUID, draft.CurrentRevisionUUID)
}

func (r *SkillDefinitionRepository) AppendRevision(
	ctx context.Context,
	tenantUUID, draftUUID, updatedByMemberUUID string,
	revision *models.SkillDefinitionRevision,
) (*models.SkillDefinitionDraft, *models.SkillDefinitionRevision, error) {
	if revision == nil {
		return nil, nil, gorm.ErrInvalidData
	}
	tenantUUID = normalizeUUID(tenantUUID)
	draftUUID = normalizeUUID(draftUUID)
	updatedByMemberUUID = normalizeUUID(updatedByMemberUUID)
	var draft models.SkillDefinitionDraft
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, draftUUID).Take(&draft).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSkillDefinitionDraftNotFound
			}
			return err
		}
		// A published revision is immutable, but the definition remains the
		// stable identity for its next revision. Appending always creates a new
		// draft revision; it never mutates the published record.
		var count int64
		if err := tx.Model(&models.SkillDefinitionRevision{}).Where("tenant_uuid = ? AND draft_uuid = ?", tenantUUID, draftUUID).Count(&count).Error; err != nil {
			return err
		}
		revision.TenantUUID = tenantUUID
		revision.DraftUUID = draftUUID
		revision.RevisionNumber = int(count) + 1
		revision.AuthoredByMemberUUID = updatedByMemberUUID
		revision.Status = models.SkillDefinitionRevisionStatusDraft
		revision.Normalize()
		if err := tx.Create(revision).Error; err != nil {
			return err
		}
		return tx.Model(&models.SkillDefinitionDraft{}).
			Where("tenant_uuid = ? AND uuid = ?", tenantUUID, draftUUID).
			Updates(map[string]any{
				"current_revision_uuid":  revision.UUID.String(),
				"updated_by_member_uuid": updatedByMemberUUID,
				"updated_at":             time.Now(),
			}).Error
	}); err != nil {
		return nil, nil, err
	}
	draft.CurrentRevisionUUID = revision.UUID.String()
	draft.UpdatedByMemberUUID = updatedByMemberUUID
	return &draft, revision, nil
}

// PublishCurrentRevision atomically makes the current revision immutable and
// records the generated package object. It does not read a local worktree or
// infer an artifact URI.
func (r *SkillDefinitionRepository) PublishCurrentRevision(
	ctx context.Context,
	tenantUUID, draftUUID, publishedArtifactURI, publishedChecksum, updatedByMemberUUID string,
) (*models.SkillDefinitionDraft, *models.SkillDefinitionRevision, error) {
	tenantUUID = normalizeUUID(tenantUUID)
	draftUUID = normalizeUUID(draftUUID)
	updatedByMemberUUID = normalizeUUID(updatedByMemberUUID)
	publishedArtifactURI = strings.TrimSpace(publishedArtifactURI)
	publishedChecksum = strings.TrimSpace(strings.ToLower(publishedChecksum))
	var draft models.SkillDefinitionDraft
	var revision models.SkillDefinitionRevision
	now := time.Now()
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, draftUUID).Take(&draft).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSkillDefinitionDraftNotFound
			}
			return err
		}
		if draft.CurrentRevisionUUID == "" {
			return ErrSkillDefinitionRevisionNotFound
		}
		if err := tx.Where("tenant_uuid = ? AND uuid = ?", tenantUUID, draft.CurrentRevisionUUID).Take(&revision).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSkillDefinitionRevisionNotFound
			}
			return err
		}
		if revision.Status != models.SkillDefinitionRevisionStatusDraft {
			return errors.New("skill.definition_revision_not_publishable")
		}
		if err := tx.Model(&models.SkillDefinitionRevision{}).
			Where("tenant_uuid = ? AND draft_uuid = ? AND status = ?", tenantUUID, draftUUID, models.SkillDefinitionRevisionStatusPublished).
			Updates(map[string]any{"status": models.SkillDefinitionRevisionStatusSuperseded}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.SkillDefinitionRevision{}).
			Where("tenant_uuid = ? AND uuid = ?", tenantUUID, revision.UUID.String()).
			Updates(map[string]any{
				"status":                 models.SkillDefinitionRevisionStatusPublished,
				"published_artifact_uri": publishedArtifactURI,
				"published_checksum":     publishedChecksum,
				"published_at":           now,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&models.SkillDefinitionDraft{}).
			Where("tenant_uuid = ? AND uuid = ?", tenantUUID, draftUUID).
			Updates(map[string]any{
				"status":                 models.SkillDefinitionDraftStatusPublished,
				"updated_by_member_uuid": updatedByMemberUUID,
				"updated_at":             now,
			}).Error
	}); err != nil {
		return nil, nil, err
	}
	draft.Status = models.SkillDefinitionDraftStatusPublished
	draft.UpdatedByMemberUUID = updatedByMemberUUID
	revision.Status = models.SkillDefinitionRevisionStatusPublished
	revision.PublishedArtifactURI = publishedArtifactURI
	revision.PublishedChecksum = publishedChecksum
	revision.PublishedAt = &now
	return &draft, &revision, nil
}

func normalizeUUID(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}
