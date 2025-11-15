package knowledge

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
)

// KnowledgeSpaceRepository 管理知识空间的持久化。
type KnowledgeSpaceRepository struct {
	*baseRepo.BaseRepository[models.KnowledgeSpace]
	db *gorm.DB
}

func NewKnowledgeSpaceRepository(db *gorm.DB) *KnowledgeSpaceRepository {
	if db == nil {
		panic("knowledge space repository requires db")
	}
	return &KnowledgeSpaceRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.KnowledgeSpace](db),
		db:             db,
	}
}

func (r *KnowledgeSpaceRepository) FindByUUID(ctx context.Context, spaceUUID uuid.UUID) (*models.KnowledgeSpace, error) {
	if spaceUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var space models.KnowledgeSpace
	if err := r.db.WithContext(ctx).Where("uuid = ?", spaceUUID).Take(&space).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &space, nil
}

func (r *KnowledgeSpaceRepository) FindByTenantAndName(ctx context.Context, tenant uuid.UUID, name string) (*models.KnowledgeSpace, error) {
	name = strings.TrimSpace(name)
	if tenant == uuid.Nil || name == "" {
		return nil, gorm.ErrInvalidData
	}
	var space models.KnowledgeSpace
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND space_name = ?", tenant, name).
		Take(&space).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &space, nil
}

// DeltaJobRepository 管理增量同步任务。
type DeltaJobRepository struct {
	*baseRepo.BaseRepository[models.DeltaJob]
	db *gorm.DB
}

func NewDeltaJobRepository(db *gorm.DB) *DeltaJobRepository {
	if db == nil {
		panic("delta job repository requires db")
	}
	return &DeltaJobRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.DeltaJob](db),
		db:             db,
	}
}

func (r *DeltaJobRepository) FindByUUID(ctx context.Context, jobUUID uuid.UUID) (*models.DeltaJob, error) {
	if jobUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var job models.DeltaJob
	if err := r.db.WithContext(ctx).Where("uuid = ?", jobUUID).Take(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &job, nil
}

// PolicyTemplateRepository 管理策略模版。
type PolicyTemplateRepository struct {
	*baseRepo.BaseRepository[models.PolicyTemplateVersion]
	db *gorm.DB
}

func NewPolicyTemplateRepository(db *gorm.DB) *PolicyTemplateRepository {
	return &PolicyTemplateRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.PolicyTemplateVersion](db),
		db:             db,
	}
}

func (r *PolicyTemplateRepository) GetByNameVersion(ctx context.Context, name, version string) (*models.PolicyTemplateVersion, error) {
	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	if name == "" || version == "" {
		return nil, gorm.ErrInvalidData
	}
	var tpl models.PolicyTemplateVersion
	err := r.db.WithContext(ctx).
		Where("template_name = ? AND version = ?", name, version).
		Take(&tpl).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &tpl, nil
}

func (r *PolicyTemplateRepository) GetByID(ctx context.Context, id uint64) (*models.PolicyTemplateVersion, error) {
	if id == 0 {
		return nil, gorm.ErrInvalidData
	}
	var tpl models.PolicyTemplateVersion
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&tpl).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &tpl, nil
}

// IngestionJobRepository 管理入库任务。
type IngestionJobRepository struct {
	*baseRepo.BaseRepository[models.IngestionJob]
	db *gorm.DB
}

func NewIngestionJobRepository(db *gorm.DB) *IngestionJobRepository {
	return &IngestionJobRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.IngestionJob](db),
		db:             db,
	}
}

func (r *IngestionJobRepository) FindByUUID(ctx context.Context, jobUUID uuid.UUID) (*models.IngestionJob, error) {
	if jobUUID == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var job models.IngestionJob
	err := r.db.WithContext(ctx).Where("uuid = ?", jobUUID).Take(&job).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &job, nil
}

// ArtifactBundleRepository 管理产物包。
type ArtifactBundleRepository struct {
	*baseRepo.BaseRepository[models.ArtifactBundle]
	db *gorm.DB
}

func NewArtifactBundleRepository(db *gorm.DB) *ArtifactBundleRepository {
	return &ArtifactBundleRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.ArtifactBundle](db),
		db:             db,
	}
}

func (r *ArtifactBundleRepository) FindByJobID(ctx context.Context, jobID uint64) (*models.ArtifactBundle, error) {
	if jobID == 0 {
		return nil, gorm.ErrInvalidData
	}
	var bundle models.ArtifactBundle
	err := r.db.WithContext(ctx).
		Where("ingestion_job_id = ?", jobID).
		Take(&bundle).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &bundle, nil
}

// FusionStrategyRepository 管理融合策略。
type FusionStrategyRepository struct {
	*baseRepo.BaseRepository[models.FusionStrategyVersion]
	db *gorm.DB
}

func NewFusionStrategyRepository(db *gorm.DB) *FusionStrategyRepository {
	return &FusionStrategyRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.FusionStrategyVersion](db),
		db:             db,
	}
}

func (r *FusionStrategyRepository) FindActiveBySpace(ctx context.Context, space uuid.UUID) (*models.FusionStrategyVersion, error) {
	if space == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var strategy models.FusionStrategyVersion
	err := r.db.WithContext(ctx).
		Where("space_uuid = ? AND deployment_state = ?", space, models.FusionDeploymentActive).
		Order("updated_at DESC").
		Take(&strategy).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &strategy, nil
}

func (r *FusionStrategyRepository) ListBySpace(ctx context.Context, space uuid.UUID, limit int) ([]*models.FusionStrategyVersion, error) {
	if space == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var strategies []*models.FusionStrategyVersion
	query := r.db.WithContext(ctx).
		Where("space_uuid = ?", space).
		Order("id DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&strategies).Error; err != nil {
		return nil, err
	}
	return strategies, nil
}

func (r *FusionStrategyRepository) FindByID(ctx context.Context, id uint64) (*models.FusionStrategyVersion, error) {
	if id == 0 {
		return nil, gorm.ErrInvalidData
	}
	var strategy models.FusionStrategyVersion
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&strategy).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &strategy, nil
}

// FeedbackCaseRepository 管理反馈案例。
type FeedbackCaseRepository struct {
	*baseRepo.BaseRepository[models.FeedbackCase]
	db *gorm.DB
}

func NewFeedbackCaseRepository(db *gorm.DB) *FeedbackCaseRepository {
	return &FeedbackCaseRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.FeedbackCase](db),
		db:             db,
	}
}

func (r *FeedbackCaseRepository) ListOpenBySpace(ctx context.Context, space uuid.UUID) ([]*models.FeedbackCase, error) {
	if space == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var cases []*models.FeedbackCase
	if err := r.db.WithContext(ctx).
		Where("space_uuid = ? AND status IN ?", space, []string{models.FeedbackStatusOpen, models.FeedbackStatusInProgress}).
		Order("sla_due_at ASC NULLS LAST").
		Find(&cases).Error; err != nil {
		return nil, err
	}
	return cases, nil
}

// DecayTaskRepository 管理衰减任务。
type DecayTaskRepository struct {
	*baseRepo.BaseRepository[models.DecayTask]
	db *gorm.DB
}

func NewDecayTaskRepository(db *gorm.DB) *DecayTaskRepository {
	if db == nil {
		panic("decay task repository requires db")
	}
	return &DecayTaskRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.DecayTask](db),
		db:             db,
	}
}

// TenantReleasePolicyRepository 管理租户灰度策略。
type TenantReleasePolicyRepository struct {
	*baseRepo.BaseRepository[models.TenantReleasePolicy]
	db *gorm.DB
}

func NewTenantReleasePolicyRepository(db *gorm.DB) *TenantReleasePolicyRepository {
	return &TenantReleasePolicyRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.TenantReleasePolicy](db),
		db:             db,
	}
}

func (r *TenantReleasePolicyRepository) FindByID(ctx context.Context, id uint64) (*models.TenantReleasePolicy, error) {
	policy := &models.TenantReleasePolicy{}
	if err := r.db.WithContext(ctx).Where("id = ?", id).Take(policy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return policy, nil
}

// TenantReleaseBatchRepository 管理灰度批次执行状态。
type TenantReleaseBatchRepository struct {
	*baseRepo.BaseRepository[models.TenantReleaseBatch]
	db *gorm.DB
}

func NewTenantReleaseBatchRepository(db *gorm.DB) *TenantReleaseBatchRepository {
	return &TenantReleaseBatchRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.TenantReleaseBatch](db),
		db:             db,
	}
}

func (r *TenantReleaseBatchRepository) FindByToken(ctx context.Context, token string) (*models.TenantReleaseBatch, error) {
	if strings.TrimSpace(token) == "" {
		return nil, gorm.ErrInvalidData
	}
	var batch models.TenantReleaseBatch
	if err := r.db.WithContext(ctx).Where("batch_token = ?", token).Take(&batch).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &batch, nil
}

func (r *TenantReleaseBatchRepository) ListByPolicyAndVersion(ctx context.Context, policyID uint64, versionID string) ([]*models.TenantReleaseBatch, error) {
	var batches []*models.TenantReleaseBatch
	if err := r.db.WithContext(ctx).
		Where("policy_id = ? AND version_id = ?", policyID, strings.TrimSpace(versionID)).
		Order("batch_index ASC").Find(&batches).Error; err != nil {
		return nil, err
	}
	return batches, nil
}

func (r *DecayTaskRepository) ListOpenBySpace(ctx context.Context, space uuid.UUID) ([]*models.DecayTask, error) {
	if space == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var tasks []*models.DecayTask
	if err := r.db.WithContext(ctx).
		Where("space_uuid = ? AND status IN ?", space, []string{"open", "assigned"}).
		Order("sla_due_at ASC").
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

// IAMSyncTaskRepository 管理 IAM 同步任务。
type IAMSyncTaskRepository struct {
	*baseRepo.BaseRepository[models.IAMSyncTask]
	db *gorm.DB
}

func NewIAMSyncTaskRepository(db *gorm.DB) *IAMSyncTaskRepository {
	return &IAMSyncTaskRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.IAMSyncTask](db),
		db:             db,
	}
}

func (r *IAMSyncTaskRepository) FetchPending(ctx context.Context, space uuid.UUID) (*models.IAMSyncTask, error) {
	if space == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var task models.IAMSyncTask
	err := r.db.WithContext(ctx).
		Where("space_uuid = ? AND status IN ?", space, []string{models.IAMSyncStatusPending, models.IAMSyncStatusRunning}).
		Order("created_at ASC").
		Take(&task).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &task, nil
}

// AuditTrailRepository 管理知识空间审计轨迹。
type AuditTrailRepository struct {
	*baseRepo.BaseRepository[models.AuditTrailEntry]
	db *gorm.DB
}

func NewAuditTrailRepository(db *gorm.DB) *AuditTrailRepository {
	return &AuditTrailRepository{
		BaseRepository: baseRepo.NewBaseRepository[models.AuditTrailEntry](db),
		db:             db,
	}
}

func (r *AuditTrailRepository) ListBySpace(ctx context.Context, space uuid.UUID, limit int) ([]*models.AuditTrailEntry, error) {
	if space == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	if limit <= 0 {
		limit = 50
	}
	var items []*models.AuditTrailEntry
	if err := r.db.WithContext(ctx).
		Where("space_uuid = ?", space).
		Order("occurred_at DESC").
		Limit(limit).
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}
