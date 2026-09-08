package agent_session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const SessionCapability = "com.corex.agent.session.manage"
const IdempotencyTTL = 24 * time.Hour

func (s *Service) CheckAccess(ctx context.Context) error {
	_, err := s.owner(ctx, SessionCapability)
	return err
}

var (
	ErrInvalid        = errors.New("AGENT_SESSION_INVALID_ARGUMENT")
	ErrUnauthorized   = errors.New("AGENT_SESSION_UNAUTHORIZED")
	ErrForbidden      = errors.New("AGENT_SESSION_FORBIDDEN")
	ErrNotFound       = errors.New("AGENT_SESSION_NOT_FOUND")
	ErrConflict       = errors.New("AGENT_SESSION_CONFLICT")
	ErrExpired        = errors.New("AGENT_SESSION_IDEMPOTENCY_EXPIRED")
	ErrContextExpired = errors.New("AGENT_SESSION_CONTEXT_EXPIRED")
	ErrDependency     = errors.New("AGENT_SESSION_UPSTREAM_DEPENDENCY")
)

type Service struct {
	repo     *repo.ServiceSessionRepository
	now      func() time.Time
	executor Executor
}

func NewService(db *gorm.DB) *Service {
	if db == nil {
		return &Service{now: time.Now}
	}
	return &Service{repo: repo.NewServiceSessionRepository(db), now: time.Now}
}

type Session struct {
	SessionUUID uuid.UUID `json:"session_uuid"`
	AgentUUID   uuid.UUID `json:"agent_uuid"`
	Title       string    `json:"title"`
	Status      string    `json:"status"`
	Revision    uint64    `json:"revision"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type Message struct {
	MessageUUID uuid.UUID `json:"message_uuid"`
	SessionUUID uuid.UUID `json:"session_uuid"`
	Role        string    `json:"role"`
	Content     string    `json:"content"`
	Sequence    uint64    `json:"sequence"`
	CreatedAt   time.Time `json:"created_at"`
}

func sessionDTO(m *m.ServiceSession) Session {
	return Session{m.UUID, m.AgentUUID, m.Title, m.Status, m.Revision, m.CreatedAt, m.UpdatedAt}
}
func messageDTO(m *m.ServiceMessage) Message {
	return Message{m.UUID, m.SessionUUID, m.Role, m.Content, m.Sequence, m.CreatedAt}
}
func translate(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	for _, known := range []error{ErrInvalid, ErrUnauthorized, ErrForbidden, ErrNotFound, ErrConflict, ErrExpired, ErrContextExpired, ErrDependency} {
		if errors.Is(err, known) {
			return known
		}
	}
	return errors.Join(ErrDependency, err)
}

func (s *Service) owner(ctx context.Context, capability string) (repo.SessionOwner, error) {
	var owner repo.SessionOwner
	if s == nil || s.repo == nil {
		return owner, ErrDependency
	}
	claims := reqctx.GetClaims(ctx)
	if claims == nil || claims.Issuer != "powerx-sts" || claims.PluginID == "" || claims.Subject == "" {
		return owner, ErrUnauthorized
	}
	if claims.ExpiresAt == nil || !s.now().Before(claims.ExpiresAt.Time) {
		return owner, ErrUnauthorized
	}
	audience := false
	for _, value := range claims.Audience {
		if value == "powerx:api" {
			audience = true
		}
	}
	if !audience {
		return owner, ErrUnauthorized
	}
	tenant, err := uuid.Parse(claims.TenantUUID)
	if err != nil || tenant == uuid.Nil {
		return owner, ErrUnauthorized
	}
	if reqctx.GetTenantUUID(ctx) != claims.TenantUUID {
		return owner, ErrUnauthorized
	}
	owner = repo.SessionOwner{TenantUUID: tenant, PluginID: claims.PluginID, ServiceActor: claims.Subject}
	published, raw, err := s.repo.GrantFacts(ctx, tenant, claims.PluginID, capability)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return owner, ErrForbidden
	}
	if err != nil {
		return owner, translate(err)
	}
	if !published {
		return owner, ErrForbidden
	}
	var credential struct {
		ClientID string   `json:"client_id"`
		Allowed  []string `json:"allowed_capabilities"`
	}
	if err = json.Unmarshal(raw, &credential); err != nil {
		return owner, translate(err)
	}
	if credential.ClientID == "" || claims.Subject != "client:"+credential.ClientID {
		return owner, ErrUnauthorized
	}
	for _, allowed := range credential.Allowed {
		if allowed == capability {
			return owner, nil
		}
	}
	return owner, ErrForbidden
}
func (s *Service) agentAllowed(ctx context.Context, owner repo.SessionOwner, id uuid.UUID) error {
	return agentAllowed(ctx, s.repo, owner, id)
}

func agentAllowed(ctx context.Context, repository *repo.ServiceSessionRepository, owner repo.SessionOwner, id uuid.UUID) error {
	agent, err := repository.Agent(ctx, owner.TenantUUID, id)
	if err != nil {
		return translate(err)
	}
	if agent.Status != "active" || agent.OwnerPluginID == nil || *agent.OwnerPluginID != owner.PluginID {
		return ErrForbidden
	}
	return nil
}
func (s *Service) Create(ctx context.Context, agentUUID uuid.UUID, title string) (Session, error) {
	owner, err := s.owner(ctx, SessionCapability)
	if err != nil {
		return Session{}, err
	}
	if agentUUID == uuid.Nil || !utf8.ValidString(title) || utf8.RuneCountInString(title) > 255 {
		return Session{}, ErrInvalid
	}
	if err = s.agentAllowed(ctx, owner, agentUUID); err != nil {
		return Session{}, err
	}
	m := &m.ServiceSession{AgentUUID: agentUUID, Title: title, Status: "active", Revision: 1}
	if err = s.repo.CreateSession(ctx, owner, m); err != nil {
		return Session{}, translate(err)
	}
	return sessionDTO(m), nil
}
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Session, error) {
	owner, err := s.owner(ctx, SessionCapability)
	if err != nil {
		return Session{}, err
	}
	if id == uuid.Nil {
		return Session{}, ErrInvalid
	}
	m, err := s.repo.GetSession(ctx, owner, id)
	if err != nil {
		return Session{}, translate(err)
	}
	return sessionDTO(m), nil
}
func (s *Service) List(ctx context.Context, page, size int) ([]Session, int64, error) {
	owner, err := s.owner(ctx, SessionCapability)
	if err != nil {
		return nil, 0, err
	}
	if page < 1 || size < 1 || size > 100 || page > 1000000 {
		return nil, 0, ErrInvalid
	}
	rows, total, err := s.repo.ListSessions(ctx, owner, page, size)
	if err != nil {
		return nil, 0, translate(err)
	}
	items := make([]Session, 0, len(rows))
	for i := range rows {
		items = append(items, sessionDTO(&rows[i]))
	}
	return items, total, nil
}

// Mutate uses an explicit transition, not a free-form status patch. Deletion
// retains an owner-scoped tombstone and repeated deletion succeeds.
func (s *Service) Mutate(ctx context.Context, id uuid.UUID, operation, title string) (Session, error) {
	owner, err := s.owner(ctx, SessionCapability)
	if err != nil {
		return Session{}, err
	}
	if id == uuid.Nil {
		return Session{}, ErrInvalid
	}
	if operation != "rename" && operation != "archive" && operation != "delete" {
		return Session{}, ErrInvalid
	}
	if !utf8.ValidString(title) || utf8.RuneCountInString(title) > 255 {
		return Session{}, ErrInvalid
	}
	var result Session
	err = s.repo.WithSession(ctx, owner, id, func(tx *repo.ServiceSessionRepository, session *m.ServiceSession) error {
		if session.Status == "deleted" {
			if operation == "delete" {
				result = sessionDTO(session)
				return nil
			}
			return ErrNotFound
		}
		if err := tx.ExpireInvocations(ctx, session, s.now()); err != nil {
			return err
		}
		active, err := tx.ActiveInvocations(ctx, session)
		if err != nil {
			return err
		}
		if active > 0 {
			return ErrConflict
		}
		switch operation {
		case "rename":
			if session.Status != "active" {
				return ErrConflict
			}
			session.Title = title
		case "archive":
			session.Status = "archived"
		case "delete":
			session.Status = "deleted"
		}
		session.Revision++
		if err := tx.SaveSession(ctx, owner, session); err != nil {
			return err
		}
		result = sessionDTO(session)
		return nil
	})
	return result, translate(err)
}
func (s *Service) Append(ctx context.Context, id uuid.UUID, key, role, content string) (Message, error) {
	owner, err := s.owner(ctx, SessionCapability)
	if err != nil {
		return Message{}, err
	}
	if id == uuid.Nil || len(key) < 1 || len(key) > 128 || strings.HasPrefix(key, "invoke:") || strings.TrimSpace(key) != key || role != "user" || strings.TrimSpace(content) == "" || len(content) > 65536 || !utf8.ValidString(content) {
		return Message{}, ErrInvalid
	}
	digest := sha256.Sum256([]byte(role + "\x00" + content))
	hash := hex.EncodeToString(digest[:])
	var result Message
	err = s.repo.WithSession(ctx, owner, id, func(tx *repo.ServiceSessionRepository, session *m.ServiceSession) error {
		if session.Status == "deleted" {
			return ErrNotFound
		}
		previous, err := tx.MessageByKey(ctx, session, key)
		if err == nil {
			if previous.RequestHash != hash {
				return ErrConflict
			}
			if !s.now().Before(previous.IdempotencyExpiresAt) {
				return ErrExpired
			}
			result = messageDTO(previous)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if session.Status != "active" {
			return ErrConflict
		}
		session.Revision++
		message := &m.ServiceMessage{Role: role, Content: content, Sequence: session.Revision, IdempotencyKey: key, RequestHash: hash, IdempotencyExpiresAt: s.now().Add(IdempotencyTTL)}
		if err := tx.InsertMessage(ctx, session, message); err != nil {
			return err
		}
		if err := tx.SaveSession(ctx, owner, session); err != nil {
			return err
		}
		result = messageDTO(message)
		return nil
	})
	return result, translate(err)
}
func (s *Service) Messages(ctx context.Context, id uuid.UUID, page, size int) ([]Message, int64, error) {
	owner, err := s.owner(ctx, SessionCapability)
	if err != nil {
		return nil, 0, err
	}
	if id == uuid.Nil || page < 1 || page > 1000000 || size < 1 || size > 100 {
		return nil, 0, ErrInvalid
	}
	rows, total, err := s.repo.ListMessages(ctx, owner, id, page, size)
	if err != nil {
		return nil, 0, translate(err)
	}
	items := make([]Message, 0, len(rows))
	for i := range rows {
		items = append(items, messageDTO(&rows[i]))
	}
	return items, total, nil
}
