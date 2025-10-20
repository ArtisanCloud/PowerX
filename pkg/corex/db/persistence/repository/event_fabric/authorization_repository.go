package eventfabric

import (
	"context"
	"errors"
	"fmt"
	"time"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	baserepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AuthorizationRepository 管理授权域的能力、Grant 与审批等持久化操作。
type AuthorizationRepository struct {
	db                  *gorm.DB
	capabilityRepo      *baserepo.BaseRepository[eventfabricmodel.AuthorizationCapability]
	grantRepo           *baserepo.BaseRepository[eventfabricmodel.AuthorizationGrant]
	grantCapabilityRepo *baserepo.BaseRepository[eventfabricmodel.AuthorizationGrantCapability]
	grantConditionRepo  *baserepo.BaseRepository[eventfabricmodel.AuthorizationGrantCondition]
	ticketRepo          *baserepo.BaseRepository[eventfabricmodel.AuthorizationApprovalTicket]
}

func NewAuthorizationRepository(db *gorm.DB) *AuthorizationRepository {
	return &AuthorizationRepository{
		db:                  db,
		capabilityRepo:      baserepo.NewBaseRepository[eventfabricmodel.AuthorizationCapability](db),
		grantRepo:           baserepo.NewBaseRepository[eventfabricmodel.AuthorizationGrant](db),
		grantCapabilityRepo: baserepo.NewBaseRepository[eventfabricmodel.AuthorizationGrantCapability](db),
		grantConditionRepo:  baserepo.NewBaseRepository[eventfabricmodel.AuthorizationGrantCondition](db),
		ticketRepo:          baserepo.NewBaseRepository[eventfabricmodel.AuthorizationApprovalTicket](db),
	}
}

func (r *AuthorizationRepository) WithDB(db *gorm.DB) *AuthorizationRepository {
	if db == nil {
		return r
	}
	return NewAuthorizationRepository(db)
}

func (r *AuthorizationRepository) DB() *gorm.DB {
	return r.db
}

// Capability operations -------------------------------------------------------

func (r *AuthorizationRepository) CreateCapability(ctx context.Context, capability *eventfabricmodel.AuthorizationCapability) (*eventfabricmodel.AuthorizationCapability, error) {
	return r.capabilityRepo.Create(ctx, capability)
}

func (r *AuthorizationRepository) UpsertCapability(ctx context.Context, capability *eventfabricmodel.AuthorizationCapability) (*eventfabricmodel.AuthorizationCapability, error) {
	unique := []clause.Column{
		{Name: "namespace"},
		{Name: "action"},
	}
	return r.capabilityRepo.Upsert(ctx, capability, unique)
}

func (r *AuthorizationRepository) GetCapabilityByUUID(ctx context.Context, id uuid.UUID) (*eventfabricmodel.AuthorizationCapability, error) {
	var record eventfabricmodel.AuthorizationCapability
	err := r.db.WithContext(ctx).
		Where("uuid = ?", id).
		Take(&record).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *AuthorizationRepository) GetCapabilityByNamespaceAction(ctx context.Context, namespace, action string) (*eventfabricmodel.AuthorizationCapability, error) {
	var record eventfabricmodel.AuthorizationCapability
	err := r.db.WithContext(ctx).
		Where("namespace = ? AND action = ?", namespace, action).
		Take(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *AuthorizationRepository) ListCapabilities(ctx context.Context, riskLevels []string) ([]*eventfabricmodel.AuthorizationCapability, error) {
	query := r.db.WithContext(ctx).Model(&eventfabricmodel.AuthorizationCapability{})
	if len(riskLevels) > 0 {
		query = query.Where("risk_level IN ?", riskLevels)
	}
	var records []*eventfabricmodel.AuthorizationCapability
	if err := query.Order("namespace, action").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *AuthorizationRepository) UpdateCapability(ctx context.Context, capability *eventfabricmodel.AuthorizationCapability) (*eventfabricmodel.AuthorizationCapability, error) {
	return r.capabilityRepo.Update(ctx, capability)
}

// Grant operations ------------------------------------------------------------

func (r *AuthorizationRepository) CreateGrant(ctx context.Context, grant *eventfabricmodel.AuthorizationGrant) (*eventfabricmodel.AuthorizationGrant, error) {
	return r.grantRepo.Create(ctx, grant)
}

func (r *AuthorizationRepository) UpdateGrant(ctx context.Context, grant *eventfabricmodel.AuthorizationGrant) (*eventfabricmodel.AuthorizationGrant, error) {
	return r.grantRepo.Update(ctx, grant)
}

func (r *AuthorizationRepository) UpdateGrantFields(ctx context.Context, grantUUID uuid.UUID, fields map[string]interface{}) error {
	if grantUUID == uuid.Nil {
		return fmt.Errorf("grant uuid is required")
	}
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&eventfabricmodel.AuthorizationGrant{}).
		Where("uuid = ?", grantUUID).
		Updates(fields).Error
}

func (r *AuthorizationRepository) IncrementGrantVersion(ctx context.Context, grantUUID uuid.UUID) error {
	if grantUUID == uuid.Nil {
		return fmt.Errorf("grant uuid is required")
	}
	return r.db.WithContext(ctx).
		Model(&eventfabricmodel.AuthorizationGrant{}).
		Where("uuid = ?", grantUUID).
		UpdateColumn("version", gorm.Expr("version + 1")).Error
}

func (r *AuthorizationRepository) GetGrantByUUID(ctx context.Context, grantUUID uuid.UUID) (*eventfabricmodel.AuthorizationGrant, error) {
	var grant eventfabricmodel.AuthorizationGrant
	err := r.db.WithContext(ctx).
		Where("uuid = ?", grantUUID).
		Take(&grant).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &grant, nil
}

func (r *AuthorizationRepository) GetGrantBySubject(ctx context.Context, tenantID uuid.UUID, subjectType string, subjectID uuid.UUID, statuses []string) ([]*eventfabricmodel.AuthorizationGrant, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("tenant id is required")
	}
	if subjectID == uuid.Nil {
		return nil, fmt.Errorf("subject id is required")
	}

	query := r.db.WithContext(ctx).
		Where("tenant_id = ? AND subject_type = ? AND subject_id = ?", tenantID, subjectType, subjectID)
	if len(statuses) > 0 {
		query = query.Where("status IN ?", statuses)
	}
	var grants []*eventfabricmodel.AuthorizationGrant
	if err := query.Order("updated_at DESC").Find(&grants).Error; err != nil {
		return nil, err
	}
	return grants, nil
}

func (r *AuthorizationRepository) ListGrants(ctx context.Context, tenantID uuid.UUID, filters map[string]interface{}, page, pageSize int) ([]*eventfabricmodel.AuthorizationGrant, int64, error) {
	query := r.db.WithContext(ctx).Model(&eventfabricmodel.AuthorizationGrant{})
	if tenantID != uuid.Nil {
		query = query.Where("tenant_id = ?", tenantID)
	}
	if v, ok := filters["status"]; ok {
		switch vv := v.(type) {
		case string:
			if vv != "" {
				query = query.Where("status = ?", vv)
			}
		case []string:
			if len(vv) > 0 {
				query = query.Where("status IN ?", vv)
			}
		}
	}
	if v, ok := filters["subject_type"]; ok {
		if s, ok := v.(string); ok && s != "" {
			query = query.Where("subject_type = ?", s)
		}
	}
	if v, ok := filters["subject_id"]; ok {
		switch vv := v.(type) {
		case uuid.UUID:
			if vv != uuid.Nil {
				query = query.Where("subject_id = ?", vv)
			}
		case string:
			if parsed, err := uuid.Parse(vv); err == nil {
				query = query.Where("subject_id = ?", parsed)
			}
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if pageSize > 0 {
		query = query.Limit(pageSize)
	}
	if page > 0 && pageSize > 0 {
		query = query.Offset((page - 1) * pageSize)
	}

	var records []*eventfabricmodel.AuthorizationGrant
	if err := query.Order("updated_at DESC").Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

// GrantCapability operations --------------------------------------------------

func (r *AuthorizationRepository) ReplaceGrantCapabilities(ctx context.Context, tx *gorm.DB, grantID uuid.UUID, capabilities []*eventfabricmodel.AuthorizationGrantCapability) error {
	if grantID == uuid.Nil {
		return fmt.Errorf("grant id is required")
	}
	if tx == nil {
		tx = r.db
	}

	if err := tx.WithContext(ctx).
		Where("grant_id = ?", grantID).
		Delete(&eventfabricmodel.AuthorizationGrantCapability{}).Error; err != nil {
		return err
	}

	if len(capabilities) == 0 {
		return nil
	}

	for _, c := range capabilities {
		c.GrantID = grantID
	}
	repo := baserepo.NewBaseRepository[eventfabricmodel.AuthorizationGrantCapability](tx)
	_, err := repo.CreateBatch(ctx, capabilities)
	return err
}

func (r *AuthorizationRepository) ListGrantCapabilities(ctx context.Context, grantID uuid.UUID) ([]*eventfabricmodel.AuthorizationGrantCapability, error) {
	if grantID == uuid.Nil {
		return nil, fmt.Errorf("grant id is required")
	}
	var items []*eventfabricmodel.AuthorizationGrantCapability
	if err := r.db.WithContext(ctx).
		Where("grant_id = ?", grantID).
		Order("created_at ASC").
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// GrantCondition operations ---------------------------------------------------

func (r *AuthorizationRepository) ReplaceGrantConditions(ctx context.Context, tx *gorm.DB, grantID uuid.UUID, conditions []*eventfabricmodel.AuthorizationGrantCondition) error {
	if grantID == uuid.Nil {
		return fmt.Errorf("grant id is required")
	}
	if tx == nil {
		tx = r.db
	}

	if err := tx.WithContext(ctx).
		Where("grant_id = ?", grantID).
		Delete(&eventfabricmodel.AuthorizationGrantCondition{}).Error; err != nil {
		return err
	}

	if len(conditions) == 0 {
		return nil
	}

	for _, cond := range conditions {
		cond.GrantID = grantID
	}
	repo := baserepo.NewBaseRepository[eventfabricmodel.AuthorizationGrantCondition](tx)
	_, err := repo.CreateBatch(ctx, conditions)
	return err
}

func (r *AuthorizationRepository) ListGrantConditions(ctx context.Context, grantID uuid.UUID) ([]*eventfabricmodel.AuthorizationGrantCondition, error) {
	if grantID == uuid.Nil {
		return nil, fmt.Errorf("grant id is required")
	}
	var items []*eventfabricmodel.AuthorizationGrantCondition
	if err := r.db.WithContext(ctx).
		Where("grant_id = ?", grantID).
		Order("created_at ASC").
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// Ticket operations -----------------------------------------------------------

func (r *AuthorizationRepository) CreateApprovalTicket(ctx context.Context, ticket *eventfabricmodel.AuthorizationApprovalTicket) (*eventfabricmodel.AuthorizationApprovalTicket, error) {
	return r.ticketRepo.Create(ctx, ticket)
}

func (r *AuthorizationRepository) UpdateApprovalTicket(ctx context.Context, ticket *eventfabricmodel.AuthorizationApprovalTicket) (*eventfabricmodel.AuthorizationApprovalTicket, error) {
	return r.ticketRepo.Update(ctx, ticket)
}

func (r *AuthorizationRepository) UpdateApprovalTicketFields(ctx context.Context, ticketUUID uuid.UUID, fields map[string]interface{}) error {
	if ticketUUID == uuid.Nil {
		return fmt.Errorf("ticket uuid is required")
	}
	if len(fields) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Model(&eventfabricmodel.AuthorizationApprovalTicket{}).
		Where("uuid = ?", ticketUUID).
		Updates(fields).Error
}

func (r *AuthorizationRepository) GetTicketByFingerprint(ctx context.Context, fingerprint uuid.UUID) (*eventfabricmodel.AuthorizationApprovalTicket, error) {
	if fingerprint == uuid.Nil {
		return nil, fmt.Errorf("fingerprint is required")
	}

	var ticket eventfabricmodel.AuthorizationApprovalTicket
	err := r.db.WithContext(ctx).
		Where("request_fingerprint = ?", fingerprint).
		Take(&ticket).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &ticket, nil
}

func (r *AuthorizationRepository) ListTicketsByStatus(ctx context.Context, tenantID uuid.UUID, statuses []string, before time.Time) ([]*eventfabricmodel.AuthorizationApprovalTicket, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("tenant id is required")
	}

	query := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID)
	if len(statuses) > 0 {
		query = query.Where("status IN ?", statuses)
	}
	if !before.IsZero() {
		query = query.Where("sla_expires_at <= ?", before)
	}

	var tickets []*eventfabricmodel.AuthorizationApprovalTicket
	if err := query.Order("sla_expires_at ASC").Find(&tickets).Error; err != nil {
		return nil, err
	}
	return tickets, nil
}

func (r *AuthorizationRepository) GetTicketByUUID(ctx context.Context, ticketUUID uuid.UUID) (*eventfabricmodel.AuthorizationApprovalTicket, error) {
	if ticketUUID == uuid.Nil {
		return nil, fmt.Errorf("ticket uuid is required")
	}
	var ticket eventfabricmodel.AuthorizationApprovalTicket
	err := r.db.WithContext(ctx).
		Where("uuid = ?", ticketUUID).
		Take(&ticket).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &ticket, nil
}

// Transaction helpers --------------------------------------------------------

func (r *AuthorizationRepository) BeginTx(ctx context.Context) (*AuthorizationRepository, *gorm.DB, error) {
	tx := r.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, nil, tx.Error
	}
	return r.WithDB(tx), tx, nil
}

func (r *AuthorizationRepository) CommitTx(tx *gorm.DB) error {
	if tx == nil {
		return nil
	}
	return tx.Commit().Error
}

func (r *AuthorizationRepository) RollbackTx(tx *gorm.DB) {
	if tx != nil {
		_ = tx.Rollback()
	}
}
