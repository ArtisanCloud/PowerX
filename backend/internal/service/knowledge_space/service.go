package knowledge_space

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/instrumentation"
	knowledge "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// RuntimeConfig captures provisioning runtime constraints.
type RuntimeConfig struct {
	LockKeyPrefix          string
	DefaultRetentionMonths int
	ProvisioningSLA        time.Duration
	EventTopics            EventTopics
}

// EventTopics defines emitted domain topics.
type EventTopics struct {
	Provisioning string
	Ingestion    string
	Fusion       string
	Feedback     string
}

// ServiceOptions aggregates dependencies.
type ServiceOptions struct {
	DB              *gorm.DB
	Instrumentation *instrumentation.Instrumentation
	Redis           redis.UniversalClient
	EventBus        event_bus.EventBus
	Config          RuntimeConfig
	Clock           func() time.Time
}

// Service implements provisioning orchestration.
type Service struct {
	db        *gorm.DB
	inst      *instrumentation.Instrumentation
	redis     redis.UniversalClient
	bus       event_bus.EventBus
	cfg       RuntimeConfig
	clock     func() time.Time
	lockTTL   time.Duration
	localMu   sync.Map
	ingestion *IngestionService
}

// NewService builds a provisioning service.
func NewService(opts ServiceOptions) *Service {
	if opts.DB == nil {
		panic("knowledge_space.Service requires DB")
	}
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	if opts.Instrumentation == nil {
		opts.Instrumentation = instrumentation.New(instrumentation.Options{})
	}
	if opts.Config.LockKeyPrefix == "" {
		opts.Config.LockKeyPrefix = "knowledge_space:lock"
	}
	if opts.Config.DefaultRetentionMonths <= 0 {
		opts.Config.DefaultRetentionMonths = 13
	}
	if opts.Config.ProvisioningSLA <= 0 {
		opts.Config.ProvisioningSLA = 2 * time.Minute
	}
	lockTTL := opts.Config.ProvisioningSLA
	if lockTTL < 30*time.Second {
		lockTTL = 30 * time.Second
	}
	return &Service{
		db:      opts.DB,
		inst:    opts.Instrumentation,
		redis:   opts.Redis,
		bus:     opts.EventBus,
		cfg:     opts.Config,
		clock:   opts.Clock,
		lockTTL: lockTTL,
	}
}

// AttachIngestion wires the ingestion service for cleanup callbacks.
func (s *Service) AttachIngestion(ingestion *IngestionService) {
	s.ingestion = ingestion
}

// CreateSpaceInput describes provisioning parameters.
type CreateSpaceInput struct {
	TenantUUID     string
	SpaceName      string
	DepartmentCode string
	QuotaCPU       int
	QuotaStorageGB int
	PolicyVersion  uint64
	FeatureFlags   []string
	RequestedBy    string
}

// UpdateSpaceInput captures mutable fields.
type UpdateSpaceInput struct {
	SpaceID        uuid.UUID
	QuotaCPU       int
	QuotaStorageGB int
	PolicyVersion  uint64
	FeatureFlags   []string
	Status         string
	UpdatedBy      string
}

// RetireSpaceInput captures retirement metadata.
type RetireSpaceInput struct {
	SpaceID     uuid.UUID
	Reason      string
	RequestedBy string
}

func (s *Service) repositories(tx *gorm.DB) (spaces *knowledge.KnowledgeSpaceRepository,
	policies *knowledge.PolicyTemplateRepository,
	iam *knowledge.IAMSyncTaskRepository,
	audit *knowledge.AuditTrailRepository,
) {
	db := s.db
	if tx != nil {
		db = tx
	}
	return knowledge.NewKnowledgeSpaceRepository(db),
		knowledge.NewPolicyTemplateRepository(db),
		knowledge.NewIAMSyncTaskRepository(db),
		knowledge.NewAuditTrailRepository(db)
}

func (s *Service) retentionDeadline(now time.Time) time.Time {
	return now.AddDate(0, s.cfg.DefaultRetentionMonths, 0)
}

func (s *Service) lockKey(tenantUUID string) string {
	return fmt.Sprintf("%s:%s", s.cfg.LockKeyPrefix, tenantUUID)
}

func (s *Service) acquireLocalLock(tenantUUID string) func() {
	key := tenantUUID
	actual, _ := s.localMu.LoadOrStore(key, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	return func() {
		mu.Unlock()
	}
}
