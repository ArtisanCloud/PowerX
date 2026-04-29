package host

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_debug/instrumentation"
	auditpkg "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"gorm.io/datatypes"
)

const (
	defaultTTLSeconds   = 600
	maxTTLSeconds       = 1800
	defaultHTTPPort     = 51701
	defaultGRPCPort     = 52701
	defaultPruneSeconds = 60
)

// HostSession represents a registered host simulator instance.
type HostSession struct {
	ID           uuid.UUID
	PluginID     string
	Environment  string
	HTTPPort     int
	GRPCPort     int
	Capabilities []string
	RuntimeID    string
	StartedAt    time.Time
	ExpiresAt    time.Time
	TTL          time.Duration
}

// Options configure host service behaviour.
type Options struct {
	Component     string
	ConfigPath    string
	PruneInterval time.Duration
	Now           func() time.Time
}

// catalog describes host simulator defaults and runtimes.
type catalog struct {
	Defaults struct {
		TTLSeconds int `yaml:"ttl_seconds"`
		HTTPPort   int `yaml:"http_port"`
		GRPCPort   int `yaml:"grpc_port"`
	} `yaml:"defaults"`
	Runtimes []struct {
		ID          string `yaml:"id"`
		Description string `yaml:"description"`
		Image       string `yaml:"image"`
		Healthcheck string `yaml:"healthcheck"`
	} `yaml:"runtimes"`
}

func loadCatalog(path string) *catalog {
	logCtx := logger.WithLogFields(context.Background(), map[string]interface{}{"module": "plugin_debug.host"})
	data := []byte{}
	if path != "" {
		b, err := os.ReadFile(path)
		if err == nil {
			data = b
		} else {
			logger.WarnF(logCtx, "[plugin_debug.host] load catalog failed: %v", err)
		}
	}
	if len(data) == 0 {
		return defaultCatalog()
	}
	var cfg catalog
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		logger.WarnF(logCtx, "[plugin_debug.host] parse catalog failed: %v", err)
		return defaultCatalog()
	}
	return &cfg
}

func defaultCatalog() *catalog {
	cfg := &catalog{}
	cfg.Defaults.TTLSeconds = defaultTTLSeconds
	cfg.Defaults.HTTPPort = defaultHTTPPort
	cfg.Defaults.GRPCPort = defaultGRPCPort
	return cfg
}

func (c *catalog) ttlDuration() time.Duration {
	ttl := c.Defaults.TTLSeconds
	if ttl <= 0 {
		ttl = defaultTTLSeconds
	}
	if ttl > maxTTLSeconds {
		ttl = maxTTLSeconds
	}
	return time.Duration(ttl) * time.Second
}

func (c *catalog) defaultHTTPPort() int {
	if c.Defaults.HTTPPort > 0 {
		return c.Defaults.HTTPPort
	}
	return defaultHTTPPort
}

func (c *catalog) defaultGRPCPort() int {
	if c.Defaults.GRPCPort > 0 {
		return c.Defaults.GRPCPort
	}
	return defaultGRPCPort
}

// Service manages host simulator registrations and telemetry.
type Service struct {
	instruments  *instrumentation.Instruments
	auditSvc     auditpkg.Service
	now          func() time.Time
	mu           sync.RWMutex
	activeHosts  map[uuid.UUID]HostSession
	catalog      *catalog
	pruneEvery   time.Duration
	stopCh       chan struct{}
	pruneOnce    sync.Once
	backgroundWG sync.WaitGroup
}

// NewService constructs the host service.
func NewService(auditSvc auditpkg.Service, opts Options) *Service {
	if opts.Component == "" {
		opts.Component = "plugin_debug"
	}
	if opts.PruneInterval <= 0 {
		opts.PruneInterval = time.Duration(defaultPruneSeconds) * time.Second
	}
	clock := opts.Now
	if clock == nil {
		clock = time.Now
	}
	svc := &Service{
		instruments: instrumentation.NewInstruments(opts.Component),
		auditSvc:    auditSvc,
		now:         clock,
		activeHosts: make(map[uuid.UUID]HostSession),
		catalog:     loadCatalog(opts.ConfigPath),
		pruneEvery:  opts.PruneInterval,
		stopCh:      make(chan struct{}),
	}
	svc.startBackground()
	return svc
}

func (s *Service) startBackground() {
	s.backgroundWG.Add(1)
	go func() {
		defer s.backgroundWG.Done()
		ticker := time.NewTicker(s.pruneEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.pruneExpired()
			case <-s.stopCh:
				return
			}
		}
	}()
}

// Close stops background pruning.
func (s *Service) Close() {
	s.pruneOnce.Do(func() {
		close(s.stopCh)
	})
	s.backgroundWG.Wait()
}

// RegisterMockHost registers a mock host session.
func (s *Service) RegisterMockHost(ctx context.Context, pluginID, environment string, ttl time.Duration, httpPort, grpcPort int, capabilities []string) HostSession {
	if ttl <= 0 {
		ttl = s.catalog.ttlDuration()
	}
	if ttl > time.Duration(maxTTLSeconds)*time.Second {
		ttl = time.Duration(maxTTLSeconds) * time.Second
	}
	if httpPort == 0 {
		httpPort = s.catalog.defaultHTTPPort()
	}
	if grpcPort == 0 {
		grpcPort = s.catalog.defaultGRPCPort()
	}
	now := s.now()
	hostID := uuid.New()
	session := HostSession{
		ID:           hostID,
		PluginID:     pluginID,
		Environment:  environment,
		HTTPPort:     httpPort,
		GRPCPort:     grpcPort,
		Capabilities: append([]string(nil), capabilities...),
		StartedAt:    now,
		ExpiresAt:    now.Add(ttl),
		TTL:          ttl,
	}

	s.mu.Lock()
	s.cleanupExpiredLocked(now)
	s.activeHosts[hostID] = session
	s.mu.Unlock()

	s.emitAudit(ctx, "PLUGIN_DEBUG_HOST_START", "INFO", hostID.String(), map[string]any{
		"plugin_id":   pluginID,
		"http_port":   httpPort,
		"grpc_port":   grpcPort,
		"environment": environment,
		"ttl_seconds": int(ttl.Seconds()),
	})
	return session
}

// StopHost removes a host session explicitly.
func (s *Service) StopHost(ctx context.Context, id uuid.UUID, reason string) (HostSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.activeHosts[id]
	if !ok {
		return HostSession{}, false
	}
	delete(s.activeHosts, id)
	s.emitAudit(ctx, "PLUGIN_DEBUG_HOST_STOP", "INFO", id.String(), map[string]any{
		"plugin_id": session.PluginID,
		"reason":    reason,
	})
	return session, true
}

// ListActive returns active host sessions.
func (s *Service) ListActive() []HostSession {
	now := s.now()
	s.mu.Lock()
	s.cleanupExpiredLocked(now)
	sessions := make([]HostSession, 0, len(s.activeHosts))
	for _, session := range s.activeHosts {
		sessions = append(sessions, session)
	}
	s.mu.Unlock()
	return sessions
}

// RecordInstall tracks install latency and audit trail.
func (s *Service) RecordInstall(ctx context.Context, event InstallEvent) {
	if s == nil {
		return
	}
	session := s.getSession(event.SessionID)
	if s.instruments != nil {
		s.instruments.RecordLatency(ctx, event.Duration)
	}
	meta := map[string]any{
		"tenant_uuid":  strings.TrimSpace(event.TenantUUID),
		"developer_id": event.DeveloperID,
		"artifact_uri": event.ArtifactURI,
		"feature_flag": event.FeatureFlag,
		"duration_ms":  event.Duration.Milliseconds(),
	}
	if session.PluginID != "" {
		meta["plugin_id"] = session.PluginID
	}
	s.emitAudit(ctx, "PLUGIN_DEBUG_INSTALL", "INFO", event.SessionID.String(), meta)
}

// RecordReload tracks hot reload metrics.
func (s *Service) RecordReload(ctx context.Context, event ReloadEvent) {
	if s == nil {
		return
	}
	session := s.getSession(event.SessionID)
	if s.instruments != nil {
		s.instruments.RecordLatency(ctx, event.Duration)
		if !event.Success {
			s.instruments.RecordFailure(ctx)
		}
		if event.VersionMismatch {
			s.instruments.RecordVersionMismatch(ctx)
		}
	}
	outcome := "INFO"
	if !event.Success {
		outcome = "WARN"
	}
	meta := map[string]any{
		"duration_ms":      event.Duration.Milliseconds(),
		"sequence":         event.Sequence,
		"version_mismatch": event.VersionMismatch,
	}
	if event.Error != "" {
		meta["error"] = event.Error
	}
	if session.PluginID != "" {
		meta["plugin_id"] = session.PluginID
	}
	s.emitAudit(ctx, "PLUGIN_DEBUG_RELOAD", outcome, event.SessionID.String(), meta)
}

func (s *Service) getSession(id uuid.UUID) HostSession {
	s.mu.RLock()
	session, ok := s.activeHosts[id]
	s.mu.RUnlock()
	if !ok {
		return HostSession{}
	}
	if s.now().After(session.ExpiresAt) {
		s.mu.Lock()
		current, still := s.activeHosts[id]
		if still && s.now().After(current.ExpiresAt) {
			delete(s.activeHosts, id)
			session = HostSession{}
		} else if still {
			session = current
		}
		s.mu.Unlock()
	}
	return session
}

func (s *Service) cleanupExpiredLocked(now time.Time) {
	for id, session := range s.activeHosts {
		if now.After(session.ExpiresAt) {
			delete(s.activeHosts, id)
		}
	}
}

func (s *Service) pruneExpired() {
	now := s.now()
	s.mu.Lock()
	s.cleanupExpiredLocked(now)
	s.mu.Unlock()
}

func (s *Service) emitAudit(ctx context.Context, operation, severity, resourceID string, meta map[string]any) {
	if s.auditSvc == nil {
		return
	}
	tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(ctx))
	event := &dbm.AuditEvent{
		OccurredAt:   s.now().UTC(),
		TenantUUID:   tenantUUID,
		Source:       "plugin_debug",
		Operation:    operation,
		ResourceType: "plugin_debug",
		ResourceID:   resourceID,
		Outcome:      severity,
		Severity:     severity,
		Meta:         marshalJSON(meta),
	}
	if err := s.auditSvc.Emit(ctx, event); err != nil {
		logger.ErrorF(ctx, "[plugin_debug.host] emit audit failed: %v", err)
	}
}

func marshalJSON(meta map[string]any) datatypes.JSON {
	if len(meta) == 0 {
		return datatypes.JSON("{}")
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return datatypes.JSON("{}")
	}
	return datatypes.JSON(data)
}

// InstallEvent stores metadata for the initial install push.
type InstallEvent struct {
	SessionID   uuid.UUID
	TenantUUID  string
	DeveloperID uint64
	ArtifactURI string
	Duration    time.Duration
	FeatureFlag string
}

// ReloadEvent stores metadata for change iterations.
type ReloadEvent struct {
	SessionID       uuid.UUID
	Duration        time.Duration
	Success         bool
	Sequence        int64
	Error           string
	VersionMismatch bool
}

// ErrHostNotFound indicates host ID is missing.
var ErrHostNotFound = errors.New("plugin_debug.host: host not found")
