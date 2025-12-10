package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/dev_hotload"
	repository "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/dev_hotload"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	// ErrNotFound indicates the session was not located.
	ErrNotFound = errors.New("dev hotload session not found")
)

// Store coordinates persistence for DevHotload sessions/events.
type Store struct {
	repo *repository.SessionRepository
	now  func() time.Time
}

func New(db *gorm.DB, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{
		repo: repository.NewSessionRepository(db),
		now:  now,
	}
}

func (s *Store) CreateSession(ctx context.Context, session *model.DevHotloadSession) error {
	return s.repo.CreateSession(ctx, session)
}

func (s *Store) SaveSession(ctx context.Context, session *model.DevHotloadSession) error {
	return s.repo.SaveSession(ctx, session)
}

func (s *Store) FindSession(ctx context.Context, id uuid.UUID) (*model.DevHotloadSession, error) {
	result, err := s.repo.FindByUUID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return result, nil
}

func (s *Store) FindActiveByPlugin(ctx context.Context, pluginID string, tenantID uint64) (*model.DevHotloadSession, error) {
	result, err := s.repo.FindActiveByPlugin(ctx, pluginID, tenantID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return result, nil
}

func (s *Store) CountActive(ctx context.Context) (int64, error) {
	return s.repo.CountActive(ctx)
}

// ListSessions returns sessions filtered by plugin/tenant/status.
func (s *Store) ListSessions(ctx context.Context, pluginID string, tenantID *uint64, statuses []string, limit, offset int) ([]model.DevHotloadSession, error) {
	filter := repository.ListSessionsFilter{
		PluginID: strings.TrimSpace(pluginID),
		TenantID: tenantID,
		Statuses: statuses,
		Limit:    limit,
		Offset:   offset,
	}
	return s.repo.ListSessions(ctx, filter)
}

func (s *Store) ListExpired(ctx context.Context, before time.Time) ([]model.DevHotloadSession, error) {
	return s.repo.ListExpired(ctx, before)
}

// DeleteSessions deletes sessions by filter and returns deleted records.
func (s *Store) DeleteSessions(ctx context.Context, pluginID string, tenantID *uint64, statuses []string) ([]model.DevHotloadSession, error) {
	filter := repository.DeleteSessionsFilter{
		PluginID: strings.TrimSpace(pluginID),
		TenantID: tenantID,
		Statuses: statuses,
	}
	return s.repo.DeleteSessions(ctx, filter)
}

func (s *Store) AppendEvent(ctx context.Context, sessionID uuid.UUID, eventType string, payload any) error {
	sequence, err := s.repo.NextEventSequence(ctx, sessionID)
	if err != nil {
		return err
	}
	data, err := marshalJSON(payload)
	if err != nil {
		return err
	}
	event := &model.DevHotloadSessionEvent{
		SessionID:  sessionID,
		EventType:  eventType,
		Payload:    data,
		Sequence:   sequence,
		OccurredAt: s.now(),
	}
	return s.repo.CreateEvent(ctx, event)
}

func marshalJSON(payload any) ([]byte, error) {
	if payload == nil {
		return []byte("{}"), nil
	}
	switch v := payload.(type) {
	case []byte:
		if len(v) == 0 {
			return []byte("{}"), nil
		}
		return v, nil
	case json.RawMessage:
		if len(v) == 0 {
			return []byte("{}"), nil
		}
		return v, nil
	default:
		return json.Marshal(v)
	}
}
