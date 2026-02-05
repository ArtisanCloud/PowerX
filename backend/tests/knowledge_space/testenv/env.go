package testenv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	agentcfg "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentsettings "github.com/ArtisanCloud/PowerX/internal/service/agent"
	knowledgeService "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	decay_guard "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/decay_guard"
	ksdelta "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/delta"
	event_hotfix "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/event_hotfix"
	knowledgeinstr "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/instrumentation"
	qaBridge "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/qa_bridge"
	tenant_release "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/tenant_release"
	knowledgegrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/knowledge_space"
	adminhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/knowledge_space"
	openapihttp "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/knowledge_space"
	knowledgeworkflow "github.com/ArtisanCloud/PowerX/internal/workflow/knowledge_space"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	settingmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Env encapsulates dependencies required to exercise Knowledge Space surfaces.
type Env struct {
	T                         testing.TB
	DB                        *gorm.DB
	Deps                      *shared.Deps
	Bus                       event_bus.EventBus
	tenantID                  uuid.UUID
	VectorStore               *VectorStoreStub
	SparseIndex               *SparseIndexStub
	Pipeline                  *ReprocessPipelineStub
	feedbackReprocessTopic    string
	feedbackUnsub             func()
	FeedbackReportPath        string
	KnowledgeUpdateReportPath string
	DeltaReportPath           string
	EventReportPath           string
	DecayReportPath           string
	ReleaseReportPath         string
	QABridgeReportPath        string
}

// New spins up an isolated sqlite + redis test environment.
func New(t testing.TB) *Env {
	t.Helper()

	if setter, ok := t.(interface{ Setenv(string, string) }); ok {
		setter.Setenv("POWERX_INGESTION_SYNC", "1")
	} else {
		_ = os.Setenv("POWERX_INGESTION_SYNC", "1")
		t.Cleanup(func() { _ = os.Unsetenv("POWERX_INGESTION_SYNC") })
	}

	gin.SetMode(gin.TestMode)

	tenantID := uuid.New()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, createAIModelProfileTable(db))
	require.NoError(t, createAIRoutePolicyTable(db))
	require.NoError(t, createAIProviderCredentialTable(db))
	require.NoError(t, createKnowledgeChunksTable(db))

	require.NoError(t, db.AutoMigrate(
		&settingmodel.TenantSetting{},
		&models.KnowledgeSpace{},
		&models.KnowledgeVectorIndex{},
		&models.PolicyTemplateVersion{},
		&models.IngestionJob{},
		&models.ArtifactBundle{},
		&models.FusionStrategyVersion{},
		&models.FeedbackCase{},
		&models.IAMSyncTask{},
		&models.AuditTrailEntry{},
		&models.SourceCredential{},
		&models.SourceConnectorInstance{},
		&models.SpaceSyncJob{},
		&models.DeltaJob{},
		&models.DecayTask{},
		&models.TenantReleasePolicy{},
		&models.TenantReleaseBatch{},
	))

	bus := event_bus.NewLocalEventBus()
	inst := knowledgeinstr.New(knowledgeinstr.Options{})
	vectorStore := NewVectorStoreStub()
	sparseIndex := NewSparseIndexStub()

	// 让入库测试在“无外部依赖”情况下也能生成向量（hash 为非语义兜底）。
	agentcfg.SetGlobalAIConfig(&agentcfg.AIConfig{
		Defaults: agentcfg.AIDefaults{
			Embedding: agentcfg.EmbeddingDefaults{
				BaseConn: agentcfg.BaseConn{
					Provider: "hash",
					Model:    "hash",
				},
				Dimensions: 32,
				Batch:      128,
				Truncate:   "none",
			},
		},
	})

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

	tempDir := filepath.Join(findProjectRoot(t), "tmp", "test-runs", uuid.NewString())
	require.NoError(t, os.MkdirAll(tempDir, 0o755))
	ingestionReportPath := filepath.Join(tempDir, "ingestion-metrics.json")
	feedbackReportPath := filepath.Join(tempDir, "knowledge-feedback.json")
	updateReportPath := filepath.Join(tempDir, "knowledge-update.json")
	deltaReportPath := filepath.Join(tempDir, "knowledge-delta.json")
	eventReportPath := filepath.Join(tempDir, "knowledge-event.json")
	releaseReportPath := filepath.Join(tempDir, "knowledge-release.json")
	decayReportPath := filepath.Join(tempDir, "knowledge-decay.json")
	deltaSourcesPath := filepath.Join(tempDir, "delta-sources.json")
	partialReleasePath := filepath.Join(tempDir, "partial-release.json")
	eventPoliciesPath := filepath.Join(tempDir, "event-policies.json")
	agentMatrixPath := filepath.Join(tempDir, "agent-weight-matrix.json")
	decayThresholdsPath := filepath.Join(tempDir, "decay-thresholds.json")
	writeSeedJSON(t, deltaSourcesPath, map[string]any{
		"sources": []map[string]any{{
			"name":     "handbook",
			"type":     "markdown",
			"endpoint": "s3://demo",
			"enabled":  true,
		}},
	})
	writeSeedJSON(t, partialReleasePath, map[string]any{
		"rules": []map[string]any{{
			"tenants": []string{"*"},
			"spaces":  []string{"*"},
		}},
	})
	writeSeedJSON(t, eventPoliciesPath, map[string]any{
		"policies": []map[string]any{{
			"eventType": "policy-update",
			"actions":   []string{"fetch", "hot-update"},
			"severity":  "p1",
		}},
	})
	writeSeedJSON(t, agentMatrixPath, map[string]any{
		"entries": map[string]any{
			"policy-update": map[string]any{"tool": "reranker", "weight": 0.9},
		},
	})
	writeSeedJSON(t, decayThresholdsPath, map[string]any{
		"thresholds": []map[string]any{{
			"category":    "coverage",
			"maxAgeHours": 48,
			"severity":    "p1",
		}},
	})
	metricsWriter := knowledgeService.NewIngestionMetricsWriter(ingestionReportPath)
	feedbackMetricsWriter := knowledgeService.NewFeedbackMetricsWriter(feedbackReportPath, updateReportPath)
	deltaMetricsWriter := knowledgeinstr.NewDeltaMetricsWriter(deltaReportPath, updateReportPath)
	eventMetricsWriter := knowledgeinstr.NewEventMetricsWriter(eventReportPath, updateReportPath)
	decayMetricsWriter := knowledgeinstr.NewDecayMetricsWriter(decayReportPath, updateReportPath)
	releaseMetricsWriter := knowledgeinstr.NewReleaseMetricsWriter(releaseReportPath, updateReportPath)
	artifactStore := knowledgeService.NewArtifactStore(knowledgeService.ArtifactStoreOptions{
		BaseDir: filepath.Join(findProjectRoot(t), "tmp", "knowledge-artifacts"),
	})
	ingestionSvc := knowledgeService.NewIngestionService(knowledgeService.IngestionServiceOptions{
		DB:              db,
		Instrumentation: inst,
		VectorStore:     vectorStore,
		MetricsWriter:   metricsWriter,
		ArtifactStore:   artifactStore,
		MaxRetries:      1,
		AgentSettings:   agentsettings.NewAgentSettingService(db),
	})
	service.AttachIngestion(ingestionSvc)

	fusionSvc := knowledgeService.NewFusionService(knowledgeService.FusionServiceOptions{
		DB:              db,
		Instrumentation: inst,
		VectorStore:     vectorStore,
		SparseIndex:     sparseIndex,
		EventBus:        bus,
		EventTopic:      cfg.EventTopics.Fusion,
		Clock:           time.Now,
	})

	reprocessTopic := cfg.EventTopics.Feedback + ".reprocess"
	pipelineInner := knowledgeworkflow.NewReprocessPipeline(knowledgeworkflow.ReprocessPipelineOptions{
		EventBus:   bus,
		EventTopic: reprocessTopic,
		Clock:      time.Now,
	})
	pipelineStub := NewReprocessPipelineStub().WithInner(pipelineInner)
	feedbackSvc := knowledgeService.NewFeedbackService(knowledgeService.FeedbackServiceOptions{
		DB:              db,
		Instrumentation: inst,
		Pipeline:        pipelineStub,
		MetricsWriter:   metricsWriter,
		FeedbackMetrics: feedbackMetricsWriter,
		Clock:           time.Now,
	})
	agentNotifier := event_hotfix.NewAgentNotifier(agentMatrixPath)
	eventHotfixSvc := event_hotfix.NewService(event_hotfix.Options{
		DB:              db,
		Instrumentation: inst,
		EventBus:        bus,
		MetricsWriter:   eventMetricsWriter,
		AgentNotifier:   agentNotifier,
		PoliciesPath:    eventPoliciesPath,
		ReportPath:      eventReportPath,
		Clock:           time.Now,
		RetryMax:        3,
		ReplayWindow:    5 * time.Minute,
	})
	deltaSvc := ksdelta.NewService(ksdelta.Options{
		DB:                       db,
		Instrumentation:          inst,
		MetricsWriter:            deltaMetricsWriter,
		SourcesConfigPath:        deltaSourcesPath,
		PartialReleaseConfigPath: partialReleasePath,
		Clock:                    time.Now,
	})
	qaBridgeReportPath := filepath.Join(t.TempDir(), "qa-reasoning.json")
	qaBridgeSvc := qaBridge.NewService(qaBridge.Options{
		DB:              db,
		Instrumentation: inst,
		VectorStore:     vectorStore,
		Clock:           time.Now,
		ReportPath:      qaBridgeReportPath,
	})
	decaySvc := decay_guard.NewService(decay_guard.Options{
		DB:              db,
		Instrumentation: inst,
		MetricsWriter:   decayMetricsWriter,
		ThresholdsPath:  decayThresholdsPath,
		Clock:           time.Now,
	})
	releaseSvc := tenant_release.NewService(tenant_release.Options{
		DB:              db,
		Instrumentation: inst,
		MetricsWriter:   releaseMetricsWriter,
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
			Ingestion:       ingestionSvc,
			Fusion:          fusionSvc,
			Feedback:        feedbackSvc,
			Delta:           deltaSvc,
			EventHotfix:     eventHotfixSvc,
			DecayGuard:      decaySvc,
			Release:         releaseSvc,
			VectorStore:     vectorStore,
			QABridge:        qaBridgeSvc,
		},
	}

	env := &Env{
		T:                         t,
		DB:                        db,
		Deps:                      deps,
		Bus:                       bus,
		tenantID:                  tenantID,
		VectorStore:               vectorStore,
		SparseIndex:               sparseIndex,
		Pipeline:                  pipelineStub,
		feedbackReprocessTopic:    reprocessTopic,
		feedbackUnsub:             nil,
		FeedbackReportPath:        feedbackReportPath,
		KnowledgeUpdateReportPath: updateReportPath,
		DeltaReportPath:           deltaReportPath,
		EventReportPath:           eventReportPath,
		DecayReportPath:           decayReportPath,
		ReleaseReportPath:         releaseReportPath,
		QABridgeReportPath:        qaBridgeReportPath,
	}
	require.NoError(t, env.seedTenantEmbeddingProfile())
	return env
}

func createAIModelProfileTable(db *gorm.DB) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS main.ai_model_profiles (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME,
  env TEXT,
  tenant_uuid TEXT,
  modality TEXT,
  provider TEXT,
  model TEXT,
  label TEXT,
  defaults TEXT,
  cap_cache TEXT,
  tags TEXT
);`
	return db.Exec(ddl).Error
}

func createAIRoutePolicyTable(db *gorm.DB) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS main.ai_route_policies (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME,
  env TEXT,
  tenant_uuid TEXT,
  modality TEXT,
  agent_id TEXT,
  flow_id TEXT,
  purpose TEXT,
  provider TEXT,
  model TEXT,
  strategy TEXT,
  compliance TEXT,
  quota TEXT
);`
	return db.Exec(ddl).Error
}

func createAIProviderCredentialTable(db *gorm.DB) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS main.ai_provider_credentials (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at DATETIME,
  updated_at DATETIME,
  deleted_at DATETIME,
  env TEXT,
  tenant_uuid TEXT,
  name TEXT,
  provider TEXT,
  auth_scheme TEXT,
  data TEXT
);`
	return db.Exec(ddl).Error
}

func createKnowledgeChunksTable(db *gorm.DB) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS main.knowledge_chunks (
  space_uuid TEXT NOT NULL,
  chunk_uuid TEXT NOT NULL,
  job_uuid TEXT,
  kind TEXT NOT NULL DEFAULT 'chunk',
  content TEXT NOT NULL,
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at DATETIME,
  updated_at DATETIME,
  PRIMARY KEY (space_uuid, chunk_uuid)
);`
	return db.Exec(ddl).Error
}

func findProjectRoot(t testing.TB) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	dir := filepath.Clean(wd)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".specify")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir || next == "" || next == "." || next == string(filepath.Separator) {
			return wd
		}
		dir = next
	}
}

func ProjectRoot(t testing.TB) string {
	return findProjectRoot(t)
}

func writeSeedJSON(t testing.TB, path string, payload any) {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

// Close releases resources created for the environment.
func (e *Env) Close() {
	if e.feedbackUnsub != nil {
		e.feedbackUnsub()
		e.feedbackUnsub = nil
	}
	if e.Bus != nil {
		_ = e.Bus.Close()
	}
}

func (e *Env) EnableFeedbackReprocessWorker() {
	if e == nil || e.feedbackUnsub != nil {
		return
	}
	if e.Bus == nil || e.DB == nil || e.VectorStore == nil {
		return
	}
	if strings.TrimSpace(e.feedbackReprocessTopic) == "" {
		return
	}
	e.feedbackUnsub = knowledgeworkflow.NewReprocessWorker(knowledgeworkflow.ReprocessWorkerOptions{
		DB:          e.DB,
		VectorStore: e.VectorStore,
		EventBus:    e.Bus,
		EventTopic:  e.feedbackReprocessTopic,
		Clock:       time.Now,
	}).Start()
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
		tenantUUID := strings.TrimSpace(c.GetHeader("X-PowerX-Tenant"))
		if tenantUUID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing X-PowerX-Tenant header"})
			return
		}
		ctx := reqctx.WithTenantUUID(c.Request.Context(), tenantUUID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	adminhttp.RegisterAPIRoutes(public, protected, e.Deps)
	openapihttp.Register(public, protected, e.Deps)
	return engine
}

// GRPCServer composes a gRPC server with the Knowledge Space service mounted.
func (e *Env) GRPCServer() *grpc.Server {
	unary := func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		ctx = e.injectTenantIntoContext(ctx)
		return handler(ctx, req)
	}
	stream := func(srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := e.injectTenantIntoContext(ss.Context())
		wrapped := &tenantAwareStream{ServerStream: ss, ctx: ctx}
		return handler(srv, wrapped)
	}
	server := grpc.NewServer(grpc.UnaryInterceptor(unary), grpc.StreamInterceptor(stream))
	knowledgegrpc.Register(server, knowledgegrpc.NewServer(e.Deps))
	return server
}

func (e *Env) injectTenantIntoContext(ctx context.Context) context.Context {
	md, _ := metadata.FromIncomingContext(ctx)
	tenantUUID := strings.TrimSpace(e.tenantID.String())
	if md != nil {
		if values := md.Get("x-tenant-uuid"); len(values) > 0 {
			if trimmed := strings.TrimSpace(values[0]); trimmed != "" {
				tenantUUID = trimmed
			}
		}
	}
	return reqctx.WithTenantUUID(ctx, tenantUUID)
}

type tenantAwareStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *tenantAwareStream) Context() context.Context {
	return s.ctx
}

// TenantUUID returns the shared tenant UUID for fixtures.
func (e *Env) TenantUUID() uuid.UUID {
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
		TenantUUID:     e.tenantID.String(),
		SpaceName:      name,
		DepartmentCode: "RD",
		QuotaCPU:       4,
		QuotaStorageGB: 120,
		PolicyVersion:  policyID,
		FeatureFlags:   []string{"ingestion.dual-chunk"},
	})
	require.NoError(e.T, err)
	require.NoError(e.T, e.ensureEmbeddingForSpace(space))
	return space
}

func (e *Env) seedTenantEmbeddingProfile() error {
	if e == nil || e.DB == nil {
		return fmt.Errorf("env not initialized")
	}
	ctx := context.Background()
	tenant := e.tenantID.String()
	provider := "hash"
	model := "hash"
	dimensions := 32
	profiles := []*agentmodel.AIModelProfile{
		{
			Env:        "dev",
			TenantUUID: &tenant,
			Modality:   "embedding",
			Provider:   provider,
			Model:      model,
			Label:      "test.embedding",
			Defaults:   datatypes.JSONMap{"dimensions": dimensions},
			CapCache: datatypes.JSONMap{
				"dimensions": dimensions,
				"probed_at":  time.Now().UTC().Format(time.RFC3339Nano),
			},
			Tags: []string{"embedding", "test"},
		},
		{
			Env:        "default",
			TenantUUID: &tenant,
			Modality:   "embedding",
			Provider:   provider,
			Model:      model,
			Label:      "test.embedding",
			Defaults:   datatypes.JSONMap{"dimensions": dimensions},
			CapCache: datatypes.JSONMap{
				"dimensions": dimensions,
				"probed_at":  time.Now().UTC().Format(time.RFC3339Nano),
			},
			Tags: []string{"embedding", "test"},
		},
	}
	for _, profile := range profiles {
		if err := e.DB.WithContext(ctx).Create(profile).Error; err != nil {
			return err
		}
	}
	return e.setTenantCurrentAIEnv(ctx, tenant, "dev")
}

func (e *Env) ensureEmbeddingForSpace(space *models.KnowledgeSpace) error {
	if e == nil || e.DB == nil || space == nil {
		return fmt.Errorf("invalid embedding setup input")
	}
	ctx := context.Background()
	tenant := e.tenantID.String()
	provider := "hash"
	model := "hash"
	dimensions := 32
	profileKey := fmt.Sprintf("%s/%s", provider, model)

	profile := &agentmodel.AIModelProfile{
		Env:        "dev",
		TenantUUID: &tenant,
		Modality:   "embedding",
		Provider:   provider,
		Model:      model,
		Label:      "test.embedding",
		Defaults:   datatypes.JSONMap{"dimensions": dimensions},
		CapCache: datatypes.JSONMap{
			"dimensions": dimensions,
			"probed_at":  time.Now().UTC().Format(time.RFC3339Nano),
		},
		Tags:       []string{"embedding", "test"},
	}
	var exist agentmodel.AIModelProfile
	err := e.DB.WithContext(ctx).
		Where("env = ? AND tenant_uuid = ? AND modality = ? AND provider = ? AND model = ?", "dev", tenant, "embedding", provider, model).
		First(&exist).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := e.DB.WithContext(ctx).Create(profile).Error; err != nil {
			return err
		}
	case err != nil:
		return err
	default:
		_ = e.DB.WithContext(ctx).Model(&exist).Updates(map[string]any{
			"defaults":  profile.Defaults,
			"cap_cache": profile.CapCache,
		}).Error
	}

	indexKey := fmt.Sprintf("dense_v1_%d", dimensions)
	tableName := fmt.Sprintf("knowledge_vectors_v1_%d", dimensions)
	index := &models.KnowledgeVectorIndex{
		SpaceUUID:           space.UUID,
		IndexKey:            indexKey,
		VectorTable:         tableName,
		Dimensions:          dimensions,
		EmbeddingProvider:   provider,
		EmbeddingModel:      model,
		EmbeddingProfileRef: profileKey,
		Status:              models.KnowledgeVectorIndexStatusActive,
	}
	if err := e.DB.WithContext(ctx).
		Where("space_uuid = ? AND index_key = ?", space.UUID, indexKey).
		FirstOrCreate(index).Error; err != nil {
		return err
	}

	return e.DB.WithContext(ctx).Model(space).Updates(map[string]any{
		"embedding_profile_key":   profileKey,
		"active_vector_index_key": indexKey,
	}).Error
}

func (e *Env) setTenantCurrentAIEnv(ctx context.Context, tenantUUID string, env string) error {
	if e == nil || e.DB == nil {
		return fmt.Errorf("env not initialized")
	}
	raw, _ := json.Marshal(strings.TrimSpace(env))
	return e.DB.WithContext(ctx).Create(&settingmodel.TenantSetting{
		TenantUUID: tenantUUID,
		Key:        agentsettings.TenantSettingKeyAICurrentEnv,
		ValueJSON:  datatypes.JSON(raw),
		Group:      "ai",
		Editable:   true,
	}).Error
}

func (e *Env) ClearSpaceEmbedding(spaceID uuid.UUID) error {
	if e == nil || e.DB == nil {
		return fmt.Errorf("env not initialized")
	}
	return e.DB.WithContext(context.Background()).
		Model(&models.KnowledgeSpace{}).
		Where("uuid = ?", spaceID).
		Updates(map[string]any{
			"embedding_profile_key":   "",
			"active_vector_index_key": "",
		}).Error
}

func (e *Env) ClearTenantEmbeddingConfig() error {
	if e == nil || e.DB == nil {
		return fmt.Errorf("env not initialized")
	}
	tenant := e.tenantID.String()
	return e.DB.WithContext(context.Background()).
		Where("tenant_uuid = ? AND modality = ?", tenant, "embedding").
		Delete(&agentmodel.AIModelProfile{}).Error
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
