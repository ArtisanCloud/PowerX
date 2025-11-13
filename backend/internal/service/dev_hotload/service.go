package devhotload

import (
	"context"
	"errors"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/dev_hotload/instrumentation"
	"github.com/ArtisanCloud/PowerX/internal/service/dev_hotload/store"
	audit "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/dev_hotload"
	"github.com/google/uuid"
)

// Service orchestrates Dev Hotload register/reload/terminate workflow.
type Service struct {
	store    *store.Store
	registry *Registry
	options  Options
	auditor  audit.Auditor
	metrics  *instrumentation.Instruments
	notifier *Notifier
}

type ServiceDeps struct {
	Store    *store.Store
	Registry *Registry
	Auditor  audit.Auditor
	Options  Options
	Metrics  *instrumentation.Instruments
	Notifier *Notifier
}

func NewService(deps ServiceDeps) *Service {
	if deps.Store == nil || deps.Registry == nil {
		panic("dev hotload service requires store and registry")
	}
	notifier := deps.Notifier
	if notifier == nil {
		notifier = NewNotifier(0)
	}
	return &Service{
		store:    deps.Store,
		registry: deps.Registry,
		options:  deps.Options,
		auditor:  deps.Auditor,
		metrics:  deps.Metrics,
		notifier: notifier,
	}
}

// RegisterInput contains metadata to start a dev session.
type RegisterInput struct {
	PluginID        string
	TenantID        uint64
	DeveloperID     uint64
	BuildHash       string
	EntryPoints     []string
	Manifest        map[string]any
	Metadata        map[string]any
	SandboxEndpoint string
	LogURL          string
	WatchFileLimit  int
}

// RegisterResult is returned to the caller for subsequent reloads.
type RegisterResult struct {
	SessionID       uuid.UUID `json:"sessionId"`
	ReloadToken     string    `json:"reloadToken"`
	Status          string    `json:"status"`
	ExpiresAt       time.Time `json:"expiresAt"`
	SandboxEndpoint string    `json:"sandboxEndpoint,omitempty"`
	LogURL          string    `json:"logUrl,omitempty"`
}

// ReloadInput holds details of a reload attempt.
type ReloadInput struct {
	SessionID   uuid.UUID
	ReloadToken string
	Sequence    int64
	Duration    time.Duration
	Changed     []string
	Artifacts   []map[string]any
	Success     bool
	Error       string
}

// Register creates a new dev hotload session.
func (s *Service) Register(ctx context.Context, input RegisterInput) (*RegisterResult, error) {
	if !s.options.FeatureFlags.Enabled {
		return nil, ErrFeatureDisabled
	}
	start := time.Now()
	session, err := s.registry.Register(ctx, RegisterRequest{
		PluginID:        input.PluginID,
		TenantID:        input.TenantID,
		DeveloperID:     input.DeveloperID,
		BuildHash:       input.BuildHash,
		EntryPoints:     input.EntryPoints,
		Manifest:        input.Manifest,
		Metadata:        input.Metadata,
		SandboxEndpoint: input.SandboxEndpoint,
		LogURL:          input.LogURL,
		WatchFileLimit:  input.WatchFileLimit,
	})
	if err != nil {
		s.recordFailure(ctx, err)
		return nil, err
	}
	if s.metrics != nil {
		s.metrics.RecordRegisterLatency(ctx, float64(time.Since(start).Milliseconds()))
		s.metrics.IncActiveSessions(ctx, 1)
	}
	s.publish(Event{
		Type:      "SessionStarted",
		SessionID: session.UUID.String(),
		Payload: map[string]any{
			"tenantId":    session.TenantID,
			"pluginId":    session.PluginID,
			"developerId": session.DeveloperID,
		},
	})
	return &RegisterResult{
		SessionID:       session.UUID,
		ReloadToken:     session.ReloadToken,
		Status:          session.Status,
		ExpiresAt:       session.ExpiresAt,
		SandboxEndpoint: session.SandboxEndpoint,
		LogURL:          session.LogURL,
	}, nil
}

// Reload records a reload event and enforces token validation.
func (s *Service) Reload(ctx context.Context, input ReloadInput) error {
	if err := s.registry.VerifyReloadToken(ctx, input.SessionID, input.ReloadToken); err != nil {
		s.recordFailure(ctx, err)
		return err
	}
	payload := map[string]any{
		"sequence":   input.Sequence,
		"success":    input.Success,
		"error":      input.Error,
		"changed":    input.Changed,
		"artifacts":  input.Artifacts,
		"durationMs": input.Duration.Milliseconds(),
	}
	if err := s.registry.RecordReload(ctx, input.SessionID, payload); err != nil {
		s.recordFailure(ctx, err)
		return err
	}
	if s.metrics != nil {
		s.metrics.RecordReloadDuration(ctx, float64(input.Duration.Milliseconds()))
	}
	s.publish(Event{
		Type:      "SessionReloaded",
		SessionID: input.SessionID.String(),
		Payload:   payload,
	})
	return nil
}

// Terminate stops a dev hotload session.
func (s *Service) Terminate(ctx context.Context, sessionID uuid.UUID, note string) error {
	if err := s.registry.Terminate(ctx, sessionID, note); err != nil {
		return err
	}
	if s.metrics != nil {
		s.metrics.IncActiveSessions(ctx, -1)
	}
	s.publish(Event{
		Type:      "SessionTerminated",
		SessionID: sessionID.String(),
		Payload:   map[string]any{"note": note},
	})
	return nil
}

// GetSession retrieves a session record for inspection.
func (s *Service) GetSession(ctx context.Context, sessionID uuid.UUID) (*model.DevHotloadSession, error) {
	return s.store.FindSession(ctx, sessionID)
}

// SubscribeEvents registers an SSE subscriber.
func (s *Service) SubscribeEvents(buffer int) (string, <-chan Event, func()) {
	return s.notifier.Subscribe(buffer)
}

// publish pushes event to SSE clients.
func (s *Service) publish(event Event) {
	if s.notifier == nil {
		return
	}
	s.notifier.Publish(event)
}

func (s *Service) recordFailure(ctx context.Context, err error) {
	if s.metrics != nil {
		s.metrics.IncFailure(ctx, classifyError(err))
	}
}

func classifyError(err error) string {
	switch {
	case errors.Is(err, ErrFeatureDisabled):
		return "feature_disabled"
	case errors.Is(err, ErrSessionConflict):
		return "session_conflict"
	case errors.Is(err, ErrReloadToken):
		return "reload_token"
	default:
		return "unknown"
	}
}
