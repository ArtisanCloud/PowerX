package testenv

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	knowledgeService "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	knowledgeinstr "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/instrumentation"
	qaBridge "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/qa_bridge"
	knowledgegrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/knowledge_space"
	adminhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/knowledge_space"
	openapihttp "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/knowledge_space"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Env encapsulates dependencies required to exercise Knowledge Space surfaces.
type Env struct {
	T           testing.TB
	DB          *gorm.DB
	Deps        *shared.Deps
	Bus         event_bus.EventBus
	tenantID    uuid.UUID
	VectorStore *VectorStoreStub
	Pipeline    *ReprocessPipelineStub
}

// New spins up an isolated sqlite + redis test environment.
func New(t testing.TB) *Env {
	t.Helper()

	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(
		&models.KnowledgeSpace{},
		&models.PolicyTemplateVersion{},
		&models.IngestionJob{},
		&models.ArtifactBundle{},
		&models.FusionStrategyVersion{},
		&models.FeedbackCase{},
		&models.IAMSyncTask{},
		&models.AuditTrailEntry{},
	))

	bus := event_bus.NewLocalEventBus()
	inst := knowledgeinstr.New(knowledgeinstr.Options{})
	vectorStore := NewVectorStoreStub()

	cfg := shared.KnowledgeSpaceRuntimeConfig{
		LockKeyPrefix:          "test:knowledge:lock",
		MetricsKeyPrefix:       "test:knowledge:metrics",
		DefaultRetentionMonths: 13,
		ProvisioningSLA:        2 * time.Minute,
		IngestionSLA:           4 * time.Hour,
		EventTopics: shared.KnowledgeSpaceEventTopicsOptions{
			Provisioning: "test.knowledge.provisioning",
			Ingestion:    "test.knowledge.ingestion",
			Fusion:       "test.knowledge.fusion",
			Feedback:     "test.knowledge.feedback",
		},
		Notifications: shared.KnowledgeSpaceNotificationOptions{
			IMWebhook:        "",
			RetryInterval:    time.Minute,
			RetryMaxAttempts: 3,
			HTTPTimeout:      5 * time.Second,
		},
	}

	serviceCfg := knowledgeService.RuntimeConfig{
		LockKeyPrefix:          cfg.LockKeyPrefix,
		DefaultRetentionMonths: cfg.DefaultRetentionMonths,
		ProvisioningSLA:        cfg.ProvisioningSLA,
		EventTopics: knowledgeService.EventTopics{
			Provisioning: cfg.EventTopics.Provisioning,
			Ingestion:    cfg.EventTopics.Ingestion,
			Fusion:       cfg.EventTopics.Fusion,
			Feedback:     cfg.EventTopics.Feedback,
		},
	}

	service := knowledgeService.NewService(knowledgeService.ServiceOptions{
		DB:              db,
		Instrumentation: inst,
		Redis:           nil,
		EventBus:        bus,
		Config:          serviceCfg,
		Clock:           time.Now,
	})

	metricsWriter := knowledgeService.NewIngestionMetricsWriter(filepath.Join(t.TempDir(), "ingestion-metrics.json"))
	ingestionSvc := knowledgeService.NewIngestionService(knowledgeService.IngestionServiceOptions{
		DB:              db,
		Instrumentation: inst,
		VectorStore:     vectorStore,
		MetricsWriter:   metricsWriter,
	})
	service.AttachIngestion(ingestionSvc)

	fusionSvc := knowledgeService.NewFusionService(knowledgeService.FusionServiceOptions{
		DB:              db,
		Instrumentation: inst,
		VectorStore:     vectorStore,
		EventBus:        bus,
		EventTopic:      cfg.EventTopics.Fusion,
		Clock:           time.Now,
	})

	pipelineStub := NewReprocessPipelineStub()
	feedbackSvc := knowledgeService.NewFeedbackService(knowledgeService.FeedbackServiceOptions{
		DB:              db,
		Instrumentation: inst,
		Pipeline:        pipelineStub,
		MetricsWriter:   metricsWriter,
		Clock:           time.Now,
	})
	qaBridgeSvc := qaBridge.NewService(qaBridge.Options{
		DB:              db,
		Instrumentation: inst,
		VectorStore:     vectorStore,
		Clock:           time.Now,
		ReportPath:      filepath.Join(t.TempDir(), "qa-reasoning.json"),
	})

	deps := &shared.Deps{
		DB:       db,
		EventBus: bus,
		KnowledgeSpace: &shared.KnowledgeSpaceDeps{
			Instrumentation: inst,
			RedisClient:     nil,
			EventBus:        bus,
			Config:          cfg,
			Service:         service,
			Ingestion:       ingestionSvc,
			Fusion:          fusionSvc,
			Feedback:        feedbackSvc,
			VectorStore:     vectorStore,
			QABridge:        qaBridgeSvc,
		},
	}

	return &Env{
		T:           t,
		DB:          db,
		Deps:        deps,
		Bus:         bus,
		tenantID:    uuid.New(),
		VectorStore: vectorStore,
		Pipeline:    pipelineStub,
	}
}

// Close releases resources created for the environment.
func (e *Env) Close() {
	if e.Bus != nil {
		_ = e.Bus.Close()
	}
}

// Engine returns a gin engine with admin routes registered.
func (e *Env) Engine() *gin.Engine {
	engine := gin.New()
	public := engine.Group("/api")
	protected := engine.Group("/api")
	protected.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	})
	adminhttp.RegisterAPIRoutes(public, protected, e.Deps)
	openapihttp.Register(public, protected, e.Deps)
	return engine
}

// GRPCServer composes a gRPC server with the Knowledge Space service mounted.
func (e *Env) GRPCServer() *grpc.Server {
	server := grpc.NewServer()
	knowledgegrpc.Register(server, knowledgegrpc.NewServer(e.Deps))
	return server
}

// TenantID returns the shared tenant ID for fixtures.
func (e *Env) TenantID() uuid.UUID {
	return e.tenantID
}

// SeedPolicyTemplate inserts a template fixture and returns its ID.
func (e *Env) SeedPolicyTemplate(name, version string) uint64 {
	tpl := &models.PolicyTemplateVersion{
		TemplateName:  name,
		Version:       version,
		ImmutableHash: uuid.NewString(),
	}
	require.NoError(e.T, e.DB.WithContext(context.Background()).Create(tpl).Error)
	return tpl.ID
}

// CreateSpaceFixture provisions a minimal active-ready knowledge space.
func (e *Env) CreateSpaceFixture(name string, policyID uint64) *models.KnowledgeSpace {
	e.T.Helper()
	require.NotNil(e.T, e.Deps)
	require.NotNil(e.T, e.Deps.KnowledgeSpace)
	svc := e.Deps.KnowledgeSpace.Service
	require.NotNil(e.T, svc)

	space, err := svc.CreateSpace(context.Background(), knowledgeService.CreateSpaceInput{
		TenantID:       e.tenantID,
		SpaceName:      name,
		DepartmentCode: "RD",
		QuotaCPU:       4,
		QuotaStorageGB: 120,
		PolicyVersion:  policyID,
		FeatureFlags:   []string{"ingestion.dual-chunk"},
	})
	require.NoError(e.T, err)
	return space
}

// ActivateSpace forces a space status to active for downstream flows.
func (e *Env) ActivateSpace(spaceID uuid.UUID) error {
	if e.Deps == nil || e.Deps.KnowledgeSpace == nil || e.Deps.KnowledgeSpace.Service == nil {
		return fmt.Errorf("knowledge space service not initialized")
	}
	_, err := e.Deps.KnowledgeSpace.Service.UpdateSpace(context.Background(), knowledgeService.UpdateSpaceInput{
		SpaceID: spaceID,
		Status:  models.KnowledgeSpaceStatusActive,
	})
	return err
}

// SetSpaceStatus updates the runtime status for a given space.
func (e *Env) SetSpaceStatus(spaceID uuid.UUID, status string) error {
	if e.Deps == nil || e.Deps.KnowledgeSpace == nil || e.Deps.KnowledgeSpace.Service == nil {
		return fmt.Errorf("knowledge space service not initialized")
	}
	if status == models.KnowledgeSpaceStatusRetired {
		_, err := e.Deps.KnowledgeSpace.Service.RetireSpace(context.Background(), knowledgeService.RetireSpaceInput{
			SpaceID: spaceID,
			Reason:  "test-retired",
		})
		return err
	}
	_, err := e.Deps.KnowledgeSpace.Service.UpdateSpace(context.Background(), knowledgeService.UpdateSpaceInput{
		SpaceID: spaceID,
		Status:  status,
	})
	return err
}
