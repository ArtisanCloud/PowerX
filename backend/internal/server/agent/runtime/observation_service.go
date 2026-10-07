package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ObservationReader is a registered, read-only adapter. It must return a
// redacted summary and/or an artifact reference, never a raw provider payload.
type ObservationReader interface {
	ReadObservation(context.Context, ResourceDescriptor, string) (ResourceObservation, error)
}

type ObservationService struct {
	repo    *repository.RuntimeObservationRepository
	mu      sync.RWMutex
	readers map[string]ObservationReader
}

type observationServiceContextKey struct{}

func ContextWithObservationService(ctx context.Context, service *ObservationService) context.Context {
	if ctx == nil || service == nil {
		return ctx
	}
	return context.WithValue(ctx, observationServiceContextKey{}, service)
}

func ObservationServiceFromContext(ctx context.Context) (*ObservationService, bool) {
	if ctx == nil {
		return nil, false
	}
	service, ok := ctx.Value(observationServiceContextKey{}).(*ObservationService)
	return service, ok && service != nil
}

// ObserveSnapshotResource is the Runtime controller action for one explicit
// resource. The caller owns the budget; this method never expands a snapshot.
func ObserveSnapshotResource(ctx context.Context, budget *RuntimeBudget, env string, resourceUUID uuid.UUID, purpose string) (ResourceObservation, error) {
	if budget == nil {
		return ResourceObservation{}, fmt.Errorf("runtime observation budget is required")
	}
	if err := budget.ConsumeObservation(); err != nil {
		return ResourceObservation{}, err
	}
	service, ok := ObservationServiceFromContext(ctx)
	if !ok {
		return ResourceObservation{}, fmt.Errorf("runtime observation service is not configured")
	}
	return service.Observe(ctx, env, resourceUUID, purpose)
}

func NewObservationService(db *gorm.DB) *ObservationService {
	return &ObservationService{repo: repository.NewRuntimeObservationRepository(db), readers: map[string]ObservationReader{}}
}
func (s *ObservationService) RegisterReader(kind string, reader ObservationReader) error {
	if s == nil || reader == nil || strings.TrimSpace(kind) == "" {
		return fmt.Errorf("observation reader kind and implementation are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readers[strings.TrimSpace(kind)] = reader
	return nil
}
func (s *ObservationService) Observe(ctx context.Context, env string, resourceUUID uuid.UUID, purpose string) (ResourceObservation, error) {
	if s == nil || s.repo == nil || strings.TrimSpace(env) == "" || strings.TrimSpace(purpose) == "" {
		return ResourceObservation{}, fmt.Errorf("observation request is incomplete")
	}
	snapshot, ok := ResourceSnapshotFromContext(ctx)
	if !ok {
		return ResourceObservation{}, ErrResourceDiscoveryDenied
	}
	resource, err := snapshot.RequireReadable(resourceUUID)
	if err != nil {
		return ResourceObservation{}, err
	}
	s.mu.RLock()
	reader := s.readers[resource.Kind]
	s.mu.RUnlock()
	if reader == nil {
		return ResourceObservation{}, fmt.Errorf("observation reader is not registered for resource kind %q", resource.Kind)
	}
	observation, err := reader.ReadObservation(ctx, resource, strings.TrimSpace(purpose))
	if err != nil {
		return ResourceObservation{}, err
	}
	if observation.ResourceUUID != resourceUUID {
		return ResourceObservation{}, fmt.Errorf("observation resource_uuid does not match request")
	}
	if observation.ObservationUUID == uuid.Nil {
		observation.ObservationUUID = uuid.New()
	}
	observation.Purpose = strings.TrimSpace(purpose)
	observation.ObservedAt = time.Now().UTC()
	directive, err := marshalReplanDirective(observation.ReplanDirective)
	if err != nil {
		return ResourceObservation{}, err
	}
	digest := sha256.Sum256(append(append([]byte(observation.Summary+"\n"+observation.ArtifactRef+"\n"), directive...), 0))
	runUUID, err := uuid.Parse(strings.TrimSpace(fmt.Sprint(ctx.Value("runtime_run_uuid"))))
	if err != nil {
		return ResourceObservation{}, fmt.Errorf("runtime_run_uuid is required")
	}
	if err := s.repo.CreateObservation(ctx, &dbmodel.AgentRunObservation{Env: strings.TrimSpace(env), TenantUUID: snapshot.TenantUUID, RunUUID: runUUID, SnapshotUUID: snapshot.SnapshotUUID, ResourceUUID: resourceUUID, Purpose: observation.Purpose, Summary: observation.Summary, ArtifactRef: observation.ArtifactRef, Directive: datatypes.JSON(directive), Digest: hex.EncodeToString(digest[:])}); err != nil {
		return ResourceObservation{}, err
	}
	return observation, nil
}

func marshalReplanDirective(directive *ReplanDirective) ([]byte, error) {
	if directive == nil {
		return []byte("{}"), nil
	}
	encoded, err := json.Marshal(directive)
	if err != nil {
		return nil, fmt.Errorf("encode observation replan directive: %w", err)
	}
	return encoded, nil
}

// RestoreObservations reconstructs persisted, redacted observation state for
// replay. Directive metadata is decoded as typed JSON, never inferred from a
// textual summary.
func (s *ObservationService) RestoreObservations(ctx context.Context, env, tenantUUID string, runUUID uuid.UUID) ([]ResourceObservation, error) {
	if s == nil || s.repo == nil || strings.TrimSpace(env) == "" || strings.TrimSpace(tenantUUID) == "" || runUUID == uuid.Nil {
		return nil, fmt.Errorf("restore observations input is incomplete")
	}
	rows, err := s.repo.ListObservations(ctx, env, tenantUUID, runUUID)
	if err != nil {
		return nil, err
	}
	out := make([]ResourceObservation, 0, len(rows))
	for _, row := range rows {
		observation := ResourceObservation{ObservationUUID: row.UUID, ResourceUUID: row.ResourceUUID, Purpose: row.Purpose, Summary: row.Summary, ArtifactRef: row.ArtifactRef, ObservedAt: row.CreatedAt.UTC()}
		if len(row.Directive) > 0 && string(row.Directive) != "{}" {
			var directive ReplanDirective
			if err := json.Unmarshal(row.Directive, &directive); err != nil {
				return nil, fmt.Errorf("decode persisted observation directive: %w", err)
			}
			observation.ReplanDirective = &directive
		}
		out = append(out, observation)
	}
	return out, nil
}
