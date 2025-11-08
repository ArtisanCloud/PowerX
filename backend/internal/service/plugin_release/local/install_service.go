package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	audit "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ErrFeatureDisabled indicates local hotload functionality is not enabled.
var ErrFeatureDisabled = errors.New("plugin_release.local_install: feature disabled")

// ErrInvalidInput represents validation failures for incoming requests.
var ErrInvalidInput = errors.New("plugin_release.local_install: invalid input")

// ErrActiveSession signals there is already an ongoing session for the tuple.
var ErrActiveSession = errors.New("plugin_release.local_install: active session exists")

// ErrSessionNotFound indicates the supplied session identifier does not match any record.
var ErrSessionNotFound = errors.New("plugin_release.local_install: session not found")

// ErrPermissionDenied indicates the actor lacks sufficient permission to hotload into a tenant.
var ErrPermissionDenied = errors.New("plugin_release.local_install: permission denied")

// ErrSignatureInvalid represents a failed signature verification.
var ErrSignatureInvalid = errors.New("plugin_release.local_install: artifact signature invalid")

// ErrArtifactTooLarge indicates the artifact exceeds configured limits.
var ErrArtifactTooLarge = errors.New("plugin_release.local_install: artifact exceeds size limit")

// SignatureVerifier encapsulates artifact signing verification.
type SignatureVerifier interface {
	Verify(ctx context.Context, tenantID uint64, artifactURI string) error
}

// PermissionChecker validates tenant/developer combinations.
type PermissionChecker interface {
	EnsureDeveloperAllowed(ctx context.Context, tenantID, developerID uint64) error
}

// CacheController coordinates cache invalidation for hotload assets.
type CacheController interface {
	ResetDeveloperCache(ctx context.Context, tenantID, developerID uint64) error
	OnSessionStarted(ctx context.Context, session *models.LocalInstallSession) error
	OnSessionStopped(ctx context.Context, sessionID uuid.UUID, status string) error
}

// ArtifactMetadata describes resolved artifact attributes.
type ArtifactMetadata struct {
	SizeBytes int64
	Checksum  string
	Signature string
}

// ArtifactMetadataResolver fetches metadata for artifacts prior to installation.
type ArtifactMetadataResolver interface {
	Resolve(ctx context.Context, tenantID uint64, artifactURI string) (*ArtifactMetadata, error)
}

// InstallServiceDeps bundles runtime dependencies required by InstallService.
type InstallServiceDeps struct {
	Repository        *repo.LocalInstallSessionRepository
	Auditor           audit.Auditor
	Clock             func() time.Time
	SignatureVerifier SignatureVerifier
	PermissionChecker PermissionChecker
	CacheController   CacheController
	MetadataResolver  ArtifactMetadataResolver
	AuditObserver     AuditHooks
}

// Options configures InstallService behaviour.
type Options struct {
	SessionTTL        time.Duration
	MaxArtifactSizeMB int
	FeatureEnabled    bool
}

// InstallService orchestrates local hotload session lifecycle.
type InstallService struct {
	repo       *repo.LocalInstallSessionRepository
	auditor    audit.Auditor
	now        func() time.Time
	db         *gorm.DB
	opts       Options
	signatures SignatureVerifier
	perm       PermissionChecker
	cache      CacheController
	metadata   ArtifactMetadataResolver
	hooks      AuditHooks
}

// StartInput contains data necessary to start a local hotload session.
type StartInput struct {
	TenantID     uint64
	DeveloperID  uint64
	ArtifactURI  string
	FeatureFlags []string
	ResetCache   bool
	Actor        string
}

// StopInput captures information for stopping a session.
type StopInput struct {
	SessionID uuid.UUID
	Force     bool
	Actor     string
}

// NewInstallService constructs the local install service with validated dependencies.
func NewInstallService(deps InstallServiceDeps, opts Options) *InstallService {
	if deps.Repository == nil {
		panic("local install service requires session repository")
	}

	svc := &InstallService{
		repo:       deps.Repository,
		auditor:    deps.Auditor,
		now:        deps.Clock,
		db:         deps.Repository.BaseRepository.DB,
		opts:       opts,
		signatures: deps.SignatureVerifier,
		perm:       deps.PermissionChecker,
		cache:      deps.CacheController,
		metadata:   deps.MetadataResolver,
		hooks:      deps.AuditObserver,
	}

	if svc.auditor == nil {
		svc.auditor = audit.Noop{}
	}
	if svc.now == nil {
		svc.now = time.Now
	}
	if svc.opts.SessionTTL <= 0 {
		svc.opts.SessionTTL = 15 * time.Minute
	}
	if svc.hooks == nil {
		svc.hooks = NoopAuditHooks{}
	}

	return svc
}

// Start launches a new local install session.
func (s *InstallService) Start(ctx context.Context, input StartInput) (*models.LocalInstallSession, error) {
	if !s.opts.FeatureEnabled {
		return nil, ErrFeatureDisabled
	}
	if err := s.validateStartInput(input); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	if s.perm != nil {
		if err := s.perm.EnsureDeveloperAllowed(ctx, input.TenantID, input.DeveloperID); err != nil {
			if errors.Is(err, ErrPermissionDenied) {
				return nil, ErrPermissionDenied
			}
			return nil, err
		}
	}

	if s.signatures != nil {
		if err := s.signatures.Verify(ctx, input.TenantID, strings.TrimSpace(input.ArtifactURI)); err != nil {
			if errors.Is(err, ErrSignatureInvalid) {
				return nil, ErrSignatureInvalid
			}
			return nil, err
		}
	}

	var artifactMeta *ArtifactMetadata
	if s.metadata != nil {
		meta, err := s.metadata.Resolve(ctx, input.TenantID, input.ArtifactURI)
		if err != nil {
			return nil, err
		}
		if meta != nil {
			if limit := s.opts.MaxArtifactSizeMB; limit > 0 {
				maxBytes := int64(limit) * 1024 * 1024
				if meta.SizeBytes > maxBytes {
					return nil, ErrArtifactTooLarge
				}
			}
			artifactMeta = meta
		}
	}

	if input.ResetCache && s.cache != nil {
		if err := s.cache.ResetDeveloperCache(ctx, input.TenantID, input.DeveloperID); err != nil {
			return nil, err
		}
	}

	// prevent duplicate in-progress sessions for same developer within tenant
	existing, err := s.repo.GetActiveSession(ctx, input.TenantID, input.DeveloperID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrActiveSession
	}

	featureFlags, err := marshalFeatureFlags(input.FeatureFlags)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	expiredAt := s.now().Add(s.opts.SessionTTL)
	logPointers, err := buildLogPointers(artifactMeta, input.ResetCache)
	if err != nil {
		return nil, err
	}

	session := &models.LocalInstallSession{
		TenantID:     input.TenantID,
		DeveloperID:  input.DeveloperID,
		ArtifactURI:  strings.TrimSpace(input.ArtifactURI),
		Status:       models.LocalInstallStatusInProgress,
		FeatureFlags: featureFlags,
		LogPointers:  logPointers,
		ExpiredAt:    &expiredAt,
	}

	created, err := s.repo.CreateSession(ctx, session)
	if err != nil {
		return nil, err
	}

	s.hooks.OnSessionStarted(ctx, created, AuditMetadata{
		ArtifactURI:  session.ArtifactURI,
		FeatureFlags: ExtractFeatureFlags(session.FeatureFlags),
		ResetCache:   input.ResetCache,
	})

	if s.cache != nil {
		if err := s.cache.OnSessionStarted(ctx, created); err != nil {
			logger.WarnF(ctx, "cache notification failed on session start: %v", err)
		}
	}

	s.recordAudit(ctx, "local_install.start", created.UUID.String(), http.StatusCreated, input.Actor)
	return created, nil
}

// Stop marks a session as terminated.
func (s *InstallService) Stop(ctx context.Context, input StopInput) error {
	if !s.opts.FeatureEnabled {
		return ErrFeatureDisabled
	}
	if input.SessionID == uuid.Nil {
		return fmt.Errorf("%w: session_id is required", ErrInvalidInput)
	}

	session, err := s.repo.GetSessionByUUID(ctx, input.SessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return ErrSessionNotFound
	}

	targetStatus := models.LocalInstallStatusSuccess
	if input.Force {
		targetStatus = models.LocalInstallStatusFailed
	}

	now := s.now()
	if err := s.repo.UpdateSessionStatus(ctx, input.SessionID, targetStatus, nil, &now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSessionNotFound
		}
		return err
	}
	session.Status = targetStatus
	session.ExpiredAt = &now

	s.hooks.OnSessionStopped(ctx, session, AuditMetadata{
		Force: input.Force,
	})

	if s.cache != nil {
		if err := s.cache.OnSessionStopped(ctx, input.SessionID, targetStatus); err != nil {
			logger.WarnF(ctx, "cache notification failed on session stop: %v", err)
		}
	}

	s.recordAudit(ctx, "local_install.stop", input.SessionID.String(), http.StatusAccepted, input.Actor)
	return nil
}

func (s *InstallService) validateStartInput(input StartInput) error {
	if input.TenantID == 0 {
		return errors.New("tenant_id must be positive")
	}
	if input.DeveloperID == 0 {
		return errors.New("developer_id must be positive")
	}
	if strings.TrimSpace(input.ArtifactURI) == "" {
		return errors.New("artifact_uri is required")
	}
	return nil
}

func marshalFeatureFlags(flags []string) (datatypes.JSON, error) {
	if len(flags) == 0 {
		return datatypes.JSON([]byte("[]")), nil
	}
	sanitised := make([]string, 0, len(flags))
	for _, flag := range flags {
		flag = strings.TrimSpace(flag)
		if flag == "" {
			continue
		}
		sanitised = append(sanitised, flag)
	}
	data, err := json.Marshal(sanitised)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(data), nil
}

func (s *InstallService) recordAudit(ctx context.Context, action, resource string, status int, actor string) {
	defer func() {
		if r := recover(); r != nil {
			logger.WarnF(ctx, "local install audit panic: %v", r)
		}
	}()

	message := fmt.Sprintf("plugin_release.%s", action)
	s.auditor.LogAPI(ctx, message, status, 0)
	if actor != "" {
		logger.InfoF(ctx, "local install action=%s actor=%s resource=%s status=%d", action, actor, resource, status)
	}
}

// ExtractFeatureFlags converts the JSON payload into a simple string slice.
func ExtractFeatureFlags(raw datatypes.JSON) []string {
	if len(raw) == 0 {
		return nil
	}
	var flags []string
	if err := json.Unmarshal(raw, &flags); err != nil {
		return nil
	}
	return flags
}

// Get retrieves a local install session by UUID.
func (s *InstallService) Get(ctx context.Context, sessionID uuid.UUID) (*models.LocalInstallSession, error) {
	if sessionID == uuid.Nil {
		return nil, fmt.Errorf("%w: session_id is required", ErrInvalidInput)
	}
	session, err := s.repo.GetSessionByUUID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

// UpdateLogPointers merges/overwrites current log pointer snapshot.
func (s *InstallService) UpdateLogPointers(ctx context.Context, sessionID uuid.UUID, pointers map[string]any) error {
	if sessionID == uuid.Nil {
		return fmt.Errorf("%w: session_id is required", ErrInvalidInput)
	}
	if len(pointers) == 0 {
		return nil
	}
	data, err := json.Marshal(pointers)
	if err != nil {
		return fmt.Errorf("encode log pointers: %w", err)
	}
	return s.repo.UpdateLogPointers(ctx, sessionID, datatypes.JSON(data))
}

func buildLogPointers(meta *ArtifactMetadata, resetCache bool) (datatypes.JSON, error) {
	state := map[string]any{}
	if meta != nil {
		if meta.Checksum != "" {
			state["artifact_checksum"] = meta.Checksum
		}
		if meta.Signature != "" {
			state["artifact_signature"] = meta.Signature
		}
		if meta.SizeBytes > 0 {
			state["artifact_size_bytes"] = meta.SizeBytes
		}
	}
	if resetCache {
		state["cache_reset"] = true
	}
	if len(state) == 0 {
		return datatypes.JSON([]byte("{}")), nil
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	return datatypes.JSON(payload), nil
}
