package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type RuntimeSnapshotService struct {
	repo *repository.RuntimeObservationRepository
}

func NewRuntimeSnapshotService(db *gorm.DB) *RuntimeSnapshotService {
	return &RuntimeSnapshotService{repo: repository.NewRuntimeObservationRepository(db)}
}

type FreezeRuntimeSnapshotInput struct {
	Env, TenantUUID, SubjectUUID, PolicyVersion string
	RunUUID, AgentUUID                          uuid.UUID
	Resources                                   []ResourceDescriptor
	ExpiresAt                                   time.Time
}

// Freeze persists only descriptors already authorized by the caller. It never
// grants read/invocation access and rejects an empty or incomplete scope.
func (s *RuntimeSnapshotService) Freeze(ctx context.Context, in FreezeRuntimeSnapshotInput) (*ResourceSnapshot, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("runtime snapshot service is not configured")
	}
	if strings.TrimSpace(in.Env) == "" || strings.TrimSpace(in.SubjectUUID) == "" || strings.TrimSpace(in.PolicyVersion) == "" || in.RunUUID == uuid.Nil || in.ExpiresAt.IsZero() {
		return nil, fmt.Errorf("runtime snapshot input is incomplete")
	}
	snapshot, err := NewResourceSnapshot(in.TenantUUID, in.AgentUUID, in.Resources, time.Now())
	if err != nil {
		return nil, err
	}
	descriptors, err := json.Marshal(snapshot.Resources)
	if err != nil {
		return nil, fmt.Errorf("marshal resource descriptors: %w", err)
	}
	digest := sha256.Sum256(descriptors)
	row := &dbmodel.AgentRunSnapshot{Env: strings.TrimSpace(in.Env), TenantUUID: snapshot.TenantUUID, RunUUID: in.RunUUID, AgentUUID: snapshot.AgentUUID, SubjectUUID: strings.TrimSpace(in.SubjectUUID), PolicyVersion: strings.TrimSpace(in.PolicyVersion), AuthorizationFingerprint: hex.EncodeToString(digest[:]), Descriptors: datatypes.JSON(descriptors), ExpiresAt: in.ExpiresAt.UTC()}
	if err := s.repo.CreateSnapshot(ctx, row); err != nil {
		return nil, err
	}
	snapshot.SnapshotUUID = row.UUID
	return snapshot, nil
}

func (s *RuntimeSnapshotService) Restore(ctx context.Context, env, tenantUUID string, runUUID, expectedSnapshotUUID uuid.UUID) (*ResourceSnapshot, error) {
	if s == nil || s.repo == nil || expectedSnapshotUUID == uuid.Nil {
		return nil, fmt.Errorf("runtime snapshot restore input is incomplete")
	}
	row, err := s.repo.GetSnapshot(ctx, env, tenantUUID, runUUID)
	if err != nil {
		return nil, err
	}
	if row.UUID != expectedSnapshotUUID || time.Now().After(row.ExpiresAt) {
		return nil, fmt.Errorf("runtime snapshot is not resumable")
	}
	var resources []ResourceDescriptor
	if err := json.Unmarshal(row.Descriptors, &resources); err != nil {
		return nil, fmt.Errorf("decode runtime snapshot descriptors: %w", err)
	}
	snapshot, err := NewResourceSnapshot(row.TenantUUID, row.AgentUUID, resources, row.CreatedAt)
	if err != nil {
		return nil, err
	}
	snapshot.SnapshotUUID = row.UUID
	return snapshot, nil
}
