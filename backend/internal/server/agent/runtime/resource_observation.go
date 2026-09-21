package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrResourceDiscoveryDenied  = errors.New("agent resource discovery denied")
	ErrResourceReadDenied       = errors.New("agent resource read denied")
	ErrResourceInvocationDenied = errors.New("agent resource invocation denied")
)

const (
	ResourceKindCapability   = "capability"
	ResourceKindAgentProfile = "agent_profile"
	ResourceKindMediaAsset   = "media_asset"
)

// ResourceDescriptor is the only resource shape a planner may observe. It
// contains no raw record payload and its UUID is the stable business identity.
type ResourceDescriptor struct {
	ResourceUUID        uuid.UUID                 `json:"resource_uuid"`
	Kind                string                    `json:"kind"`
	DisplayName         string                    `json:"display_name"`
	Description         string                    `json:"description,omitempty"`
	TenantUUID          string                    `json:"tenant_uuid"`
	Source              string                    `json:"source"`
	Classification      string                    `json:"classification"`
	CapabilityUUID      uuid.UUID                 `json:"capability_uuid,omitempty"`
	CapabilityID        string                    `json:"capability_id,omitempty"`
	DiscoveryGranted    bool                      `json:"discovery_granted"`
	ReadGranted         bool                      `json:"read_granted"`
	InvocationGranted   bool                      `json:"invocation_granted"`
	RiskLevel           string                    `json:"risk_level,omitempty"`
	RuntimeContract     CapabilityRuntimeContract `json:"runtime_contract,omitempty"`
	FreshnessObservedAt time.Time                 `json:"freshness_observed_at"`
}

type ResourceDiscoveryRequest struct {
	TenantUUID string
	AgentUUID  uuid.UUID
	UserUUID   string
	Limit      int
}

type ResourceReadRequest struct {
	TenantUUID   string
	AgentUUID    uuid.UUID
	UserUUID     string
	ResourceUUID uuid.UUID
	Purpose      string
}

// ResourceObservation is a redacted, auditable read result. Raw data remains
// owned by the resource provider and must be stored as an Artifact reference.
type ResourceObservation struct {
	ObservationUUID uuid.UUID        `json:"observation_uuid"`
	ResourceUUID    uuid.UUID        `json:"resource_uuid"`
	Purpose         string           `json:"purpose"`
	Summary         string           `json:"summary,omitempty"`
	ArtifactRef     string           `json:"artifact_ref,omitempty"`
	ReplanDirective *ReplanDirective `json:"replan_directive,omitempty"`
	ObservedAt      time.Time        `json:"observed_at"`
}

// ReplanDirective is emitted only by a registered reader as structured
// metadata. Runtime never extracts it from Summary or any other free-form
// model/provider text.
type ReplanDirective struct {
	Action         string         `json:"action"`
	CapabilityUUID uuid.UUID      `json:"capability_uuid"`
	TaskID         string         `json:"task_id"`
	DependsOn      []string       `json:"depends_on,omitempty"`
	Params         map[string]any `json:"params,omitempty"`
}

// ResourceObservationCatalog separates discovery from read. Invocation remains
// owned by CapabilityInvocationService and cannot be obtained through this API.
type ResourceObservationCatalog interface {
	Discover(ctx context.Context, request ResourceDiscoveryRequest) ([]ResourceDescriptor, error)
	Read(ctx context.Context, request ResourceReadRequest) (ResourceObservation, error)
}

// ResourceSnapshot freezes a bounded, authorized view for one Agent Run.
// Re-planning can only reference descriptors contained in this snapshot.
type ResourceSnapshot struct {
	SnapshotUUID uuid.UUID            `json:"snapshot_uuid"`
	TenantUUID   string               `json:"tenant_uuid"`
	AgentUUID    uuid.UUID            `json:"agent_uuid"`
	CreatedAt    time.Time            `json:"created_at"`
	Resources    []ResourceDescriptor `json:"resources"`
	byUUID       map[uuid.UUID]ResourceDescriptor
}

type resourceSnapshotContextKey struct{}
type runtimeObservationsContextKey struct{}

func ContextWithResourceSnapshot(ctx context.Context, snapshot *ResourceSnapshot) context.Context {
	if ctx == nil || snapshot == nil {
		return ctx
	}
	return context.WithValue(ctx, resourceSnapshotContextKey{}, snapshot)
}

func ResourceSnapshotFromContext(ctx context.Context) (*ResourceSnapshot, bool) {
	if ctx == nil {
		return nil, false
	}
	snapshot, ok := ctx.Value(resourceSnapshotContextKey{}).(*ResourceSnapshot)
	return snapshot, ok && snapshot != nil
}

// ContextWithRuntimeObservations exposes only redacted observation output to
// subsequent planning/execution. Resource descriptors and raw provider data
// remain outside this context value.
func ContextWithRuntimeObservations(ctx context.Context, observations []ResourceObservation) context.Context {
	if ctx == nil {
		return ctx
	}
	copyOf := make([]ResourceObservation, len(observations))
	copy(copyOf, observations)
	return context.WithValue(ctx, runtimeObservationsContextKey{}, copyOf)
}

func RuntimeObservationsFromContext(ctx context.Context) []ResourceObservation {
	if ctx == nil {
		return nil
	}
	observations, ok := ctx.Value(runtimeObservationsContextKey{}).([]ResourceObservation)
	if !ok {
		return nil
	}
	copyOf := make([]ResourceObservation, len(observations))
	copy(copyOf, observations)
	return copyOf
}

func NewResourceSnapshot(tenantUUID string, agentUUID uuid.UUID, resources []ResourceDescriptor, now time.Time) (*ResourceSnapshot, error) {
	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" {
		return nil, fmt.Errorf("tenant_uuid is required")
	}
	if agentUUID == uuid.Nil {
		return nil, fmt.Errorf("agent_uuid is required")
	}
	if now.IsZero() {
		return nil, fmt.Errorf("snapshot time is required")
	}
	snapshot := &ResourceSnapshot{
		SnapshotUUID: uuid.New(),
		TenantUUID:   tenantUUID,
		AgentUUID:    agentUUID,
		CreatedAt:    now.UTC(),
		Resources:    make([]ResourceDescriptor, 0, len(resources)),
		byUUID:       make(map[uuid.UUID]ResourceDescriptor, len(resources)),
	}
	for _, resource := range resources {
		if err := validateResourceDescriptor(tenantUUID, resource); err != nil {
			return nil, err
		}
		if !resource.DiscoveryGranted {
			return nil, ErrResourceDiscoveryDenied
		}
		if _, exists := snapshot.byUUID[resource.ResourceUUID]; exists {
			return nil, fmt.Errorf("duplicate resource_uuid: %s", resource.ResourceUUID)
		}
		snapshot.byUUID[resource.ResourceUUID] = resource
		snapshot.Resources = append(snapshot.Resources, resource)
	}
	sort.Slice(snapshot.Resources, func(i, j int) bool {
		return snapshot.Resources[i].ResourceUUID.String() < snapshot.Resources[j].ResourceUUID.String()
	})
	return snapshot, nil
}

func (s *ResourceSnapshot) RequireReadable(resourceUUID uuid.UUID) (ResourceDescriptor, error) {
	resource, err := s.require(resourceUUID)
	if err != nil {
		return ResourceDescriptor{}, err
	}
	if !resource.ReadGranted {
		return ResourceDescriptor{}, ErrResourceReadDenied
	}
	return resource, nil
}

func (s *ResourceSnapshot) RequireInvocable(resourceUUID uuid.UUID) (ResourceDescriptor, error) {
	resource, err := s.require(resourceUUID)
	if err != nil {
		return ResourceDescriptor{}, err
	}
	if !resource.InvocationGranted {
		return ResourceDescriptor{}, ErrResourceInvocationDenied
	}
	return resource, nil
}

func (s *ResourceSnapshot) require(resourceUUID uuid.UUID) (ResourceDescriptor, error) {
	if s == nil || resourceUUID == uuid.Nil {
		return ResourceDescriptor{}, ErrResourceDiscoveryDenied
	}
	resource, ok := s.byUUID[resourceUUID]
	if !ok {
		return ResourceDescriptor{}, ErrResourceDiscoveryDenied
	}
	return resource, nil
}

func validateResourceDescriptor(tenantUUID string, resource ResourceDescriptor) error {
	if resource.ResourceUUID == uuid.Nil {
		return fmt.Errorf("resource_uuid is required")
	}
	if strings.TrimSpace(resource.Kind) == "" {
		return fmt.Errorf("resource kind is required")
	}
	if strings.TrimSpace(resource.DisplayName) == "" {
		return fmt.Errorf("resource display_name is required")
	}
	if !strings.EqualFold(strings.TrimSpace(resource.TenantUUID), tenantUUID) {
		return fmt.Errorf("resource tenant_uuid does not match snapshot")
	}
	if resource.Kind == ResourceKindCapability && resource.CapabilityUUID == uuid.Nil {
		return fmt.Errorf("capability resource requires capability_uuid")
	}
	return nil
}
