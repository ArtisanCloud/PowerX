package bootstrap

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	archiveops "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	storagegov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	storagehttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/storage"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	authmw "github.com/ArtisanCloud/PowerX/pkg/auth/middleware"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestStorageLocalConfiguredAcceptance 是显式本机验收，不启动 Core、worker 或监听端口。
// 仅本模块的真实占用样本会保留；草稿保存／读回／冲突测试在独立事务内回滚。
func TestStorageLocalConfiguredAcceptance(t *testing.T) {
	path := os.Getenv("POWERX_STORAGE_LOCAL_ACCEPTANCE_CONFIG")
	if path == "" {
		t.Skip("opt-in existing local development configuration")
	}
	require.True(t, filepath.IsAbs(path), "验收必须显式指定绝对配置路径")
	t.Chdir(filepath.Dir(filepath.Dir(path))) // 原 make dev 从 backend 运行；go test 默认目录是 internal/bootstrap。
	cfg, err := config.LoadForMigration(path)
	require.NoError(t, err)
	require.Equal(t, "dev", cfg.Deployment.Env, "验收仅支持本地 dev")
	require.NotEmpty(t, cfg.EnvironmentID, "必须复用现有安装身份，不能创建另一份实例 ID")
	require.NotEmpty(t, cfg.Database.DSN, "验收需要现有显式 PostgreSQL DSN")
	connection, err := pgx.ParseConfig(cfg.Database.DSN)
	require.NoError(t, err)
	require.Contains(t, []string{"localhost", "127.0.0.1", "::1"}, connection.Host, "禁止连接远程")
	require.Equal(t, "powerx", connection.Database, "验收限定本地 powerx 库")
	c := cfg.StorageGovernance
	require.Equal(t, "observe", c.Mode)
	require.NotEmpty(t, c.HostID)
	require.NotEmpty(t, c.FilesystemID)
	require.NotEmpty(t, c.PostgresClusterID)
	require.True(t, c.PostgresEnabled)
	require.True(t, c.SamplingEnabled)
	require.False(t, config.BackgroundWorkersDisabled(cfg), "当前配置不能禁用后台采样")
	db, err := gorm.Open(postgres.Open(cfg.Database.DSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	run, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var cluster string
	require.NoError(t, db.WithContext(run).Raw("SELECT 'pg-' || system_identifier::text FROM pg_control_system()").Scan(&cluster).Error)
	require.Equal(t, c.PostgresClusterID, cluster, "共享集群标识必须对应当前连接")
	var rootID uint64
	require.NoError(t, db.WithContext(run).Raw("SELECT id FROM public.iam_user WHERE is_root AND deleted_at IS NULL ORDER BY id LIMIT 1").Scan(&rootID).Error)
	require.NotZero(t, rootID)
	backupDirectory := backupops.NewJobService(db).RuntimeSettings().ArtifactDirectory
	makeDeps := func(database *gorm.DB) *shared.Deps {
		deps := &shared.Deps{DB: database}
		require.NoError(t, configureStorageGovernance(cfg, deps, archiveops.Config{}, errors.New("archive adapter not part of this acceptance"), backupDirectory))
		return deps
	}
	// 真实部署配置、JWT 中间件和正式 Storage 模块路由；令牌密钥只存在本测试内。
	secret := make([]byte, 32)
	_, err = rand.Read(secret)
	require.NoError(t, err)
	claims := reqctx.CoreXClaims{UserID: rootID, IsRoot: true, TenantUUID: "00000000-0000-4000-8000-000000000001", MemberUUID: "00000000-0000-4000-8000-000000000002"}
	rootToken, err := auth.GenerateAccessJWT(claims, "storage-local-acceptance", []string{"admin"}, time.Minute, secret)
	require.NoError(t, err)
	claims.IsRoot = false
	memberToken, err := auth.GenerateAccessJWT(claims, "storage-local-acceptance", []string{"admin"}, time.Minute, secret)
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	request := func(deps *shared.Deps, token, method, suffix, body string) *httptest.ResponseRecorder {
		r := gin.New()
		group := r.Group("/api/v1", authmw.JwtMiddleware(secret, "storage-local-acceptance", []string{"admin"}, []string{"access"}, nil))
		storagehttp.RegisterAPIRoutes(r.Group("/public"), group, deps)
		req := httptest.NewRequest(method, "/api/v1/admin/ops/storage/"+suffix, strings.NewReader(body)).WithContext(run)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		r.ServeHTTP(out, req)
		return out
	}
	deps := makeDeps(db)
	observation := request(deps, rootToken, "GET", "overview", "")
	require.Equal(t, 200, observation.Code)
	var observed struct {
		Data storagegov.Overview `json:"data"`
	}
	require.NoError(t, json.Unmarshal(observation.Body.Bytes(), &observed))
	require.Equal(t, "observed", observed.Data.Filesystem.State)
	require.Equal(t, c.HostID, observed.Data.Identity.HostID)
	require.Equal(t, c.FilesystemID, observed.Data.Identity.FilesystemID)
	require.NotNil(t, observed.Data.Database)
	require.Equal(t, "observed", observed.Data.Database.State)
	require.NotNil(t, observed.Data.Database.DatabaseBytes)
	require.NotNil(t, observed.Data.Database.WALBytes)
	require.Len(t, observed.Data.Database.Relations, 2)
	for _, relation := range observed.Data.Database.Relations {
		require.Equal(t, "observed", relation.State)
		require.NotNil(t, relation.TotalBytes)
	}
	require.False(t, observed.Data.CleanupEnabled)
	require.True(t, observed.Data.Sampling.Configured)
	require.True(t, observed.Data.Sampling.WorkerEnabled)
	require.Equal(t, 403, request(deps, memberToken, "GET", "overview", "").Code)
	rollback := errors.New("rollback acceptance draft")
	var policyRevision int64
	err = db.WithContext(run).Transaction(func(tx *gorm.DB) error {
		testDeps := makeDeps(tx)
		read := request(testDeps, rootToken, "GET", "policy", "")
		require.Equal(t, 200, read.Code)
		var current struct {
			Data storagegov.PolicyDocument `json:"data"`
		}
		require.NoError(t, json.Unmarshal(read.Body.Bytes(), &current))
		policyRevision = current.Data.Revision
		input := storagegov.PolicyInput{SchemaVersion: 1, ExpectedRevision: policyRevision, State: "draft", Budget: storagegov.HostBudget{MaxManagedBytes: 90 << 30, MinFreeBytes: 20 << 30, CriticalFreeBytes: 10 << 30}, Resources: []storagegov.ResourcePolicy{{ResourceID: "core-backups", TargetDays: 3, MaxBytes: 1 << 30, RetentionCount: 24, MinVerifiedPoints: 1, ProtectLatest: true, RequireVerifiedArchive: true}}}
		raw, e := json.Marshal(input)
		require.NoError(t, e)
		write := request(testDeps, rootToken, "PUT", "policy", string(raw))
		require.Equal(t, 200, write.Code, write.Body.String())
		readback := request(testDeps, rootToken, "GET", "policy", "")
		var saved struct {
			Data storagegov.PolicyDocument `json:"data"`
		}
		require.NoError(t, json.Unmarshal(readback.Body.Bytes(), &saved))
		require.Equal(t, policyRevision+1, saved.Data.Revision)
		require.Equal(t, input.Resources, saved.Data.Resources)
		require.False(t, saved.Data.CleanupEnabled)
		require.Equal(t, 409, request(testDeps, rootToken, "PUT", "policy", string(raw)).Code)
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	afterRollback, err := deps.StorageGovernance.Policy(run)
	require.NoError(t, err)
	require.Equal(t, policyRevision, afterRollback.Revision)
	// 保留一份真实当前占用摘要，遵守正常每实例每五分钟幂等和 15 天元数据保留。
	require.NoError(t, deps.StorageGovernance.CaptureSample(run))
	var samples int64
	require.NoError(t, db.WithContext(run).Model(&coremodel.Sample{}).Where("tenant_id = ? AND instance_id = ?", 0, cfg.EnvironmentID).Count(&samples).Error)
	require.Positive(t, samples)
	history := request(deps, rootToken, "GET", "history", "")
	require.Equal(t, 200, history.Code)
	var historical struct {
		Data storagegov.ObservationHistory `json:"data"`
	}
	require.NoError(t, json.Unmarshal(history.Body.Bytes(), &historical))
	require.Nil(t, historical.Data.DailyIncrementBytes)
	t.Logf("local acceptance: PostgreSQL database/2 relations/WAL observed; draft save/readback/conflict verified and rolled back; actual occupancy records=%d completed-window records=%d; cleanup=false; user services untouched", samples, historical.Data.SampleCount)
}

// 本机只读验收：不保存草稿、不捕获样本、不触发任何文件清理或后台 worker。
func TestStorageLocalInventoryReadOnlyAcceptance(t *testing.T) {
	path := os.Getenv("POWERX_STORAGE_LOCAL_ACCEPTANCE_CONFIG")
	if path == "" {
		t.Skip("opt-in local read-only inventory")
	}
	require.True(t, filepath.IsAbs(path))
	t.Chdir(filepath.Dir(filepath.Dir(path)))
	if alias := os.Getenv("POWERX_STORAGE_LOCAL_ACCEPTANCE_PWD"); alias != "" {
		actual, err := os.Stat(".")
		require.NoError(t, err)
		aliased, err := os.Stat(alias)
		require.NoError(t, err)
		require.True(t, os.SameFile(actual, aliased), "只允许同一实际工作目录的别名")
		t.Setenv("PWD", alias)
		cwd, err := os.Getwd()
		require.NoError(t, err)
		require.Equal(t, alias, cwd)
		t.Log("read-only acceptance reproduces shell PWD alias")
	}
	cfg, err := config.LoadForMigration(path)
	require.NoError(t, err)
	require.Equal(t, "dev", cfg.Deployment.Env)
	conn, err := pgx.ParseConfig(cfg.Database.DSN)
	require.NoError(t, err)
	require.Contains(t, []string{"localhost", "127.0.0.1", "::1"}, conn.Host)
	require.Equal(t, "powerx", conn.Database)
	db, err := gorm.Open(postgres.Open(cfg.Database.DSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	defer pool.Close()
	var rootID uint64
	require.NoError(t, db.Raw("SELECT id FROM public.iam_user WHERE is_root AND deleted_at IS NULL ORDER BY id LIMIT 1").Scan(&rootID).Error)
	require.NotZero(t, rootID)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ctx = reqctx.WithIsRoot(ctx, true)
	ctx = reqctx.WithClaims(ctx, &reqctx.CoreXClaims{UserID: rootID, IsRoot: true})
	deps := &shared.Deps{DB: db}
	dir := backupops.NewJobService(db).RuntimeSettings().ArtifactDirectory
	require.NoError(t, configureStorageGovernance(cfg, deps, archiveops.Config{}, errors.New("archive not included in this read-only probe"), dir))
	out, err := deps.StorageGovernance.Overview(ctx)
	require.NoError(t, err)
	logs := 0
	for _, r := range out.Resources {
		t.Logf("source=%s state=%s files=%d partial_bytes=%d problem=%s", r.ID, r.State, r.Files, r.PartialBytes, r.Problem)
		if r.ID == "core-agent-debug" || r.ID == "core-audit-files" || r.ID == "core-local-media" {
			if _, err := os.Stat(r.Path); err == nil {
				require.NotEqual(t, "unavailable", r.State, "存在的本地来源必须可以打开")
			}
		}
		if strings.HasPrefix(r.ID, "core-app-logs-") {
			logs++
			require.Equal(t, cfg.EnvironmentID, r.InstanceID)
			require.Equal(t, "dev", r.Scope)
			require.Equal(t, "observed", r.State)
			require.Greater(t, r.Files, int64(0))
			t.Logf("actual application logs: state=%s files=%d", r.State, r.Files)
		}
	}
	require.Greater(t, logs, 0)
	p, err := deps.StorageGovernance.Policy(ctx)
	require.NoError(t, err)
	controls := 0
	for _, o := range p.Catalog {
		if o.Cleanup != nil && o.Cleanup.Supported {
			controls++
			require.Equal(t, cfg.LogConfig.File.MaxAge, o.Cleanup.RetentionDays)
			require.Equal(t, cfg.LogConfig.File.MaxBackups, o.Cleanup.RetentionCount)
		}
	}
	require.Equal(t, logs, controls)
	h, err := deps.StorageGovernance.History(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, h.Sources)
	records, err := deps.StorageGovernance.CleanupRuns(ctx)
	require.NoError(t, err)
	t.Logf("registered sources=%d history points=%d cleanup audit records=%d; no writes performed", len(out.Resources), len(h.Points), len(records))
}

// TestStorageLocalRuntimePreparedReadOnlyAcceptance 核对真实配置的组装结果，不启动服务或调度。
// PostgreSQL 连接强制只读；仅运行目录初始化允许落盘，不修改业务行或策略。
func TestStorageLocalRuntimePreparedReadOnlyAcceptance(t *testing.T) {
	path := os.Getenv("POWERX_STORAGE_LOCAL_ACCEPTANCE_CONFIG")
	if path == "" {
		t.Skip("opt-in prepared local runtime configuration")
	}
	require.True(t, filepath.IsAbs(path))
	t.Chdir(filepath.Dir(filepath.Dir(path)))
	cfg, err := config.LoadForMigration(path)
	require.NoError(t, err)
	require.Equal(t, "dev", cfg.Deployment.Env)
	require.NotEmpty(t, cfg.EnvironmentID, "必须沿用现有安装身份")
	connection, err := pgx.ParseConfig(cfg.Database.DSN)
	require.NoError(t, err)
	require.Contains(t, []string{"localhost", "127.0.0.1", "::1"}, connection.Host)
	require.Equal(t, uint16(5432), connection.Port)
	require.Equal(t, "powerx", connection.Database)
	connection.RuntimeParams["default_transaction_read_only"] = "on"
	// ConnString 保留解析时的原始字符串；通过 connector 才会应用新增的只读运行参数。
	readonlyPool := sql.OpenDB(stdlib.GetConnector(*connection))
	t.Cleanup(func() { require.NoError(t, readonlyPool.Close()) })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: readonlyPool}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var readOnly string
	require.NoError(t, db.WithContext(ctx).Raw("SHOW transaction_read_only").Scan(&readOnly).Error)
	require.Equal(t, "on", readOnly)
	var rootID uint64
	require.NoError(t, db.WithContext(ctx).Raw("SELECT id FROM public.iam_user WHERE is_root AND deleted_at IS NULL ORDER BY id LIMIT 1").Scan(&rootID).Error)
	require.NotZero(t, rootID)
	ctx = reqctx.WithIsRoot(reqctx.WithClaims(ctx, &reqctx.CoreXClaims{UserID: rootID, IsRoot: true}), true)
	deps := &shared.Deps{DB: db}
	directory := backupops.NewJobService(db).RuntimeSettings().ArtifactDirectory
	require.NoError(t, configureStorageGovernance(cfg, deps, archiveops.Config{}, errors.New("archive transfer excluded from readiness probe"), directory))
	runtime, err := deps.StorageGovernance.Runtime(ctx)
	require.NoError(t, err)
	require.True(t, runtime.Configured)
	require.True(t, runtime.WorkerEnabled, "只表示新进程允许启动 worker，本测试不启动调度")
	require.True(t, runtime.AdmissionEnabled)
	require.Empty(t, runtime.Problem)
	require.NotNil(t, runtime.Lifecycle)
	require.True(t, runtime.Lifecycle.AllowEnforce)
	for _, policy := range runtime.Lifecycle.Policies {
		require.False(t, policy.Enabled, "准备运行环境不能自动启用文件删除")
	}
	status, err := deps.StorageGovernance.DatabaseRetention(ctx)
	require.NoError(t, err)
	require.True(t, status.StateConfigured)
	require.True(t, status.EnforceAllowed)
	require.True(t, status.MigrationReady, status.MigrationProblem)
	require.True(t, status.Ready, status.Problem)
	require.False(t, status.Policy.Enabled)
	require.False(t, status.Policy.RejectNewWhenFull)
	require.Empty(t, status.Runs)
	require.NotNil(t, status.RelationBytes)
	require.True(t, cfg.LogConfig.File.Enable, "继续写应用日志")
	require.True(t, cfg.Audit.File.Enable, "继续写审计日志")
	var statement, collector string
	require.NoError(t, db.WithContext(ctx).Raw("SHOW log_statement").Scan(&statement).Error)
	require.NoError(t, db.WithContext(ctx).Raw("SHOW logging_collector").Scan(&collector).Error)
	require.Equal(t, "mod", statement)
	require.Equal(t, "on", collector)
	t.Log("local runtime assembly ready; PostgreSQL session read-only; file/database policies off; application/audit/SQL logging enabled; no workers or user services started")
}
