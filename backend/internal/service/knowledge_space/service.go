package knowledge_space

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/instrumentation"
	strategy_catalog "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/strategy_catalog"
	knowledge "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

// RuntimeConfig captures provisioning runtime constraints.
type RuntimeConfig struct {
	LockKeyPrefix            string
	DefaultRetentionMonths   int
	ProvisioningSLA          time.Duration
	EventTopics              EventTopics
	SceneStrategyCatalogPath string
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
	db              *gorm.DB
	inst            *instrumentation.Instrumentation
	redis           redis.UniversalClient
	bus             event_bus.EventBus
	cfg             RuntimeConfig
	clock           func() time.Time
	lockTTL         time.Duration
	localMu         sync.Map
	ingestion       *IngestionService
	strategyCatalog *strategy_catalog.Loader
}

func resolveSceneStrategyCatalogPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	candidates := make([]string, 0, 32)
	if strings.HasPrefix(path, "backend/") {
		candidates = append(candidates, strings.TrimPrefix(path, "backend/"))
	} else {
		candidates = append(candidates, filepath.Join("backend", path))
	}
	// 兼容 go test 的工作目录在 package 目录内（例如 backend/tests/...），向上回退尝试定位文件。
	for i := 0; i < 8; i++ {
		prefix := strings.Repeat(".."+string(filepath.Separator), i)
		candidates = append(candidates, filepath.Clean(filepath.Join(prefix, path)))
		for _, alt := range candidates[:1] {
			candidates = append(candidates, filepath.Clean(filepath.Join(prefix, alt)))
		}
	}
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return path
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
	if opts.Config.SceneStrategyCatalogPath == "" {
		opts.Config.SceneStrategyCatalogPath = "backend/config/knowledge/scene_strategy_catalog.yaml"
	}
	opts.Config.SceneStrategyCatalogPath = resolveSceneStrategyCatalogPath(opts.Config.SceneStrategyCatalogPath)
	lockTTL := opts.Config.ProvisioningSLA
	if lockTTL < 30*time.Second {
		lockTTL = 30 * time.Second
	}
	return &Service{
		db:              opts.DB,
		inst:            opts.Instrumentation,
		redis:           opts.Redis,
		bus:             opts.EventBus,
		cfg:             opts.Config,
		clock:           opts.Clock,
		lockTTL:         lockTTL,
		strategyCatalog: strategy_catalog.NewLoader(opts.Config.SceneStrategyCatalogPath),
	}
}

// AttachIngestion wires the ingestion service for cleanup callbacks.
func (s *Service) AttachIngestion(ingestion *IngestionService) {
	s.ingestion = ingestion
}

// CreateSpaceInput describes provisioning parameters.
type CreateSpaceInput struct {
	TenantUUID          string
	SpaceName           string
	DepartmentCode      string
	QuotaCPU            int
	QuotaStorageGB      int
	PolicyVersion       uint64
	IngestionProfileKey string
	IndexProfileKey     string
	RAGProfileKey       string
	FeatureFlags        []string
	RequestedBy         string
}

// UpdateSpaceInput captures mutable fields.
type UpdateSpaceInput struct {
	SpaceID             uuid.UUID
	QuotaCPU            int
	QuotaStorageGB      int
	PolicyVersion       uint64
	IngestionProfileKey string
	IndexProfileKey     string
	RAGProfileKey       string
	FeatureFlags        []string
	Status              string
	UpdatedBy           string
}

// RetireSpaceInput captures retirement metadata.
type RetireSpaceInput struct {
	SpaceID     uuid.UUID
	Reason      string
	RequestedBy string
	DropVectors bool
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
