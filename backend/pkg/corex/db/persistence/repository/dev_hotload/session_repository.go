package devhotload

import (
	"context"
	"strings"
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/dev_hotload"
	baseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SessionRepository provides CRUD helpers for DevHotload sessions.
type SessionRepository struct {
	*baseRepo.BaseRepository[model.DevHotloadSession]
	eventsRepo *baseRepo.BaseRepository[model.DevHotloadSessionEvent]
	db         *gorm.DB
}

// ListSessionsFilter scopes plugin/tenant/status queries.
type ListSessionsFilter struct {
	PluginID string
	TenantID *uint64
	Statuses []string
	Limit    int
	Offset   int
}

// NewSessionRepository constructs a repository compliant with CRUD ruleset.
func NewSessionRepository(db *gorm.DB) *SessionRepository {
	if db == nil {
		panic("dev hotload session repository requires non-nil db")
	}
	return &SessionRepository{
		BaseRepository: baseRepo.NewBaseRepository[model.DevHotloadSession](db),
		eventsRepo:     baseRepo.NewBaseRepository[model.DevHotloadSessionEvent](db),
		db:             db,
	}
}

// CreateSession persists a new session entity.
func (r *SessionRepository) CreateSession(ctx context.Context, session *model.DevHotloadSession) error {
	if session == nil {
		return gorm.ErrInvalidData
	}
	_, err := r.BaseRepository.Create(ctx, session)
	return err
}

// SaveSession updates the provided session.
func (r *SessionRepository) SaveSession(ctx context.Context, session *model.DevHotloadSession) error {
	if session == nil {
		return gorm.ErrInvalidData
	}
	return r.db.WithContext(ctx).Save(session).Error
}

// FindByUUID fetches a session by UUID.
func (r *SessionRepository) FindByUUID(ctx context.Context, id uuid.UUID) (*model.DevHotloadSession, error) {
	if id == uuid.Nil {
		return nil, gorm.ErrInvalidData
	}
	var session model.DevHotloadSession
	if err := r.db.WithContext(ctx).First(&session, "uuid = ?", id).Error; err != nil {
		return nil, err
	}
	return &session, nil
}

// FindActiveByPlugin returns the latest active session scoped to plugin + tenant.
func (r *SessionRepository) FindActiveByPlugin(ctx context.Context, pluginID string, tenantID uint64) (*model.DevHotloadSession, error) {
	var session model.DevHotloadSession
	err := r.db.WithContext(ctx).
		Where("plugin_id = ? AND tenant_id = ? AND status IN ?", pluginID, tenantID,
			[]string{model.DevHotloadSessionStatusPending, model.DevHotloadSessionStatusActive},
		).
		Order("created_at DESC").
		First(&session).
		Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// CountActive counts active sessions; caller enforces tenant scope if需要.
func (r *SessionRepository) CountActive(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.DevHotloadSession{}).
		Where("status IN ?", []string{model.DevHotloadSessionStatusPending, model.DevHotloadSessionStatusActive}).
		Count(&count).Error
	return count, err
}

// ListSessions returns sessions filtered by plugin, tenant, and statuses.
func (r *SessionRepository) ListSessions(ctx context.Context, filter ListSessionsFilter) ([]model.DevHotloadSession, error) {
	query := r.db.WithContext(ctx).Model(&model.DevHotloadSession{})
	if plugin := strings.TrimSpace(filter.PluginID); plugin != "" {
		query = query.Where("plugin_id = ?", plugin)
	}
	if filter.TenantID != nil && *filter.TenantID > 0 {
		query = query.Where("tenant_id = ?", *filter.TenantID)
	}
	if len(filter.Statuses) > 0 {
		query = query.Where("status IN ?", filter.Statuses)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}
	var sessions []model.DevHotloadSession
	err := query.Order("created_at DESC").Limit(limit).Find(&sessions).Error
	return sessions, err
}

// ListExpired returns sessions whose expiration is before provided timestamp.
func (r *SessionRepository) ListExpired(ctx context.Context, before time.Time) ([]model.DevHotloadSession, error) {
	var sessions []model.DevHotloadSession
	err := r.db.WithContext(ctx).
		Where("expires_at <= ? AND status IN ?", before, []string{model.DevHotloadSessionStatusPending, model.DevHotloadSessionStatusActive}).
		Find(&sessions).Error
	return sessions, err
}

// CreateEvent appends a session event.
func (r *SessionRepository) CreateEvent(ctx context.Context, event *model.DevHotloadSessionEvent) error {
	if event == nil {
		return gorm.ErrInvalidData
	}
	_, err := r.eventsRepo.Create(ctx, event)
	return err
}

// NextEventSequence returns the next incremental sequence for session events.
func (r *SessionRepository) NextEventSequence(ctx context.Context, sessionID uuid.UUID) (int64, error) {
	if sessionID == uuid.Nil {
		return 0, gorm.ErrInvalidData
	}
	var seq struct {
		Value int64
	}
	err := r.db.WithContext(ctx).
		Table((model.DevHotloadSessionEvent{}).TableName()).
		Select("COALESCE(MAX(sequence), 0) as value").
		Where("session_id = ?", sessionID).
		Scan(&seq).Error
	return seq.Value + 1, err
}
