package testenv

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	knowledgeService "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	knowledgeinstr "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/instrumentation"
	knowledgegrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/knowledge_space"
	adminhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/knowledge_space"
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
	T        testing.TB
	DB       *gorm.DB
	Deps     *shared.Deps
	Bus      event_bus.EventBus
	tenantID uuid.UUID
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

	deps := &shared.Deps{
		DB:       db,
		EventBus: bus,
		KnowledgeSpace: &shared.KnowledgeSpaceDeps{
			Instrumentation: inst,
			RedisClient:     nil,
			EventBus:        bus,
			Config:          cfg,
			Service:         service,
		},
	}

	return &Env{
		T:        t,
		DB:       db,
		Deps:     deps,
		Bus:      bus,
		tenantID: uuid.New(),
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
