package pluginreleaseintegration

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	pluginreleasepb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/plugin_release/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	plugsvc "github.com/ArtisanCloud/PowerX/internal/service/plugin_release"
	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/local"
	httpopenapi "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/plugin_release"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const bufSize = 1024 * 1024

type pluginReleaseEnv struct {
	DB      *gorm.DB
	Deps    *shared.Deps
	Service *plugsvc.Service
}

func newPluginReleaseEnv(t *testing.T) *pluginReleaseEnv {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&parseTime=true&_loc=UTC"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("ATTACH DATABASE ':memory:' AS public").Error)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(
		&models.LocalInstallSession{},
		&models.PluginReleaseCandidate{},
		&models.ReleasePlan{},
		&models.CanaryDeploymentRecord{},
		&models.OfflineDistributionPackage{},
		&models.MarketplaceListing{},
	))
	require.NoError(t, db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_plugin_release_candidate_tenant_plugin_version ON plugin_release_candidates(tenant_id, plugin_id, version)").Error)

	candidateRepo := repo.NewReleaseCandidateRepository(db)
	planRepo := repo.NewReleasePlanRepository(db)
	distributionRepo := repo.NewDistributionRepository(db)
	sessionRepo := repo.NewLocalInstallSessionRepository(db)

	service := plugsvc.NewService(
		candidateRepo,
		planRepo,
		distributionRepo,
		sessionRepo,
		"test.plugin.release",
		plugsvc.Options{
			FeatureFlags: plugsvc.FeatureFlagOptions{
				EnableLocalInstall:        true,
				EnableOfflineDistribution: true,
			},
			LocalInstall: plugsvc.LocalInstallOptions{
				SessionTTL:        10 * time.Minute,
				MaxArtifactSizeMB: 50,
			},
			Runtime: plugsvc.RuntimeOptions{
				RollbackTimeout: 5 * time.Minute,
			},
			Distribution: plugsvc.DistributionOptions{
				OfflineBucket:       "test-offline",
				OfflinePrefix:       "packages",
				EscalationThreshold: 2,
				ArtifactRetention:   30 * 24 * time.Hour,
				ReviewSLA:           48 * time.Hour,
			},
		},
	)

	deps := &shared.Deps{
		PluginReleaseService: service,
		PluginReleaseOptions: shared.PluginReleaseOptions{
			FeatureFlags: shared.PluginReleaseFeatureFlagsOptions{
				EnableLocalInstall:        true,
				EnableOfflineDistribution: true,
			},
			LocalInstall: shared.PluginReleaseLocalInstallOptions{
				SessionTTL:        10 * time.Minute,
				MaxArtifactSizeMB: 50,
			},
			Distribution: shared.PluginReleaseDistributionOptions{
				OfflineBucket:       "test-offline",
				OfflinePrefix:       "packages",
				EscalationThreshold: 2,
				ArtifactRetention:   30 * 24 * time.Hour,
			},
		},
	}

	return &pluginReleaseEnv{
		DB:      db,
		Deps:    deps,
		Service: service,
	}
}

type pluginReleaseServer struct {
	pluginreleasepb.UnimplementedPluginReleaseServiceServer
	svc *plugsvc.Service
}

func (s *pluginReleaseServer) StartLocalInstall(ctx context.Context, req *pluginreleasepb.StartLocalInstallRequest) (*pluginreleasepb.LocalInstallSession, error) {
	tenantID, err := strconv.ParseUint(strings.TrimSpace(req.GetTenantId()), 10, 64)
	if err != nil || tenantID == 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid tenant_id")
	}
	session, err := s.svc.LocalInstall().Start(ctx, local.StartInput{
		TenantID:     tenantID,
		DeveloperID:  req.GetDeveloperId(),
		ArtifactURI:  req.GetArtifactUri(),
		FeatureFlags: req.GetFeatureFlags(),
		ResetCache:   req.GetResetCache(),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	resp := &pluginreleasepb.LocalInstallSession{
		SessionId:   session.UUID.String(),
		TenantId:    strconv.FormatUint(session.TenantID, 10),
		DeveloperId: session.DeveloperID,
		ArtifactUri: session.ArtifactURI,
		FeatureFlags: func() []string {
			flags := local.ExtractFeatureFlags(session.FeatureFlags)
			if flags == nil {
				return []string{}
			}
			return flags
		}(),
		Status: session.Status,
	}
	if !session.CreatedAt.IsZero() {
		resp.CreatedAt = session.CreatedAt.UTC().Format(time.RFC3339)
	}
	if session.ExpiredAt != nil {
		resp.ExpiresAt = session.ExpiredAt.UTC().Format(time.RFC3339)
	}
	return resp, nil
}

func TestLocalHotloadFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	env := newPluginReleaseEnv(t)

	listener := bufconn.Listen(bufSize)
	server := grpc.NewServer()
	pluginreleasepb.RegisterPluginReleaseServiceServer(server, &pluginReleaseServer{svc: env.Service})
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := pluginreleasepb.NewPluginReleaseServiceClient(conn)
	startResp, err := client.StartLocalInstall(ctx, &pluginreleasepb.StartLocalInstallRequest{
		TenantId:     "101",
		DeveloperId:  2025,
		ArtifactUri:  "s3://bucket/hotload.zip",
		FeatureFlags: []string{"beta_ui"},
		ResetCache:   true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, startResp.GetSessionId())
	require.Equal(t, models.LocalInstallStatusInProgress, startResp.GetStatus())

	engine := gin.New()
	group := engine.Group("/api")
	httpopenapi.RegisterTenantRoutes(group, env.Deps)

	getReq := httptest.NewRequest(http.MethodGet, "/api/tenant/plugin-release/local/sessions/"+startResp.GetSessionId(), nil)
	getReq.Header.Set("Authorization", "Bearer admin")
	getResp := httptest.NewRecorder()
	engine.ServeHTTP(getResp, getReq)
	require.Equal(t, http.StatusOK, getResp.Code)

	req := httptest.NewRequest(http.MethodDelete, "/api/tenant/plugin-release/local/sessions/"+startResp.GetSessionId(), nil)
	req.Header.Set("Authorization", "Bearer admin")
	resp := httptest.NewRecorder()
	engine.ServeHTTP(resp, req)
	require.Equal(t, http.StatusAccepted, resp.Code)

	var stored struct {
		Status    string
		ExpiredAt string
	}
	require.NoError(t, env.DB.WithContext(context.Background()).
		Table("plugin_release_local_install_sessions").
		Select("status, expired_at").
		Where("uuid = ?", startResp.GetSessionId()).
		Scan(&stored).Error)

	require.Equal(t, models.LocalInstallStatusSuccess, stored.Status)
	require.NotEmpty(t, stored.ExpiredAt)

	afterStopReq := httptest.NewRequest(http.MethodGet, "/api/tenant/plugin-release/local/sessions/"+startResp.GetSessionId(), nil)
	afterStopReq.Header.Set("Authorization", "Bearer admin")
	afterStopResp := httptest.NewRecorder()
	engine.ServeHTTP(afterStopResp, afterStopReq)
	require.Equal(t, http.StatusOK, afterStopResp.Code)
}
