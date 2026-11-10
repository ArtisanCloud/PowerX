package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	taskqueue "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	media "github.com/ArtisanCloud/PowerX/internal/service/media"
	host "github.com/ArtisanCloud/PowerX/internal/testutil/runtime_host"
	ef "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	"github.com/ArtisanCloud/PowerX/pkg/instanceidentity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestBackfillCommandRejectsInvalidTenantOrBatchesBeforeConnecting(t *testing.T) {
	for _, args := range [][]string{{"--tenant", "x"}, {"--tenant", "x", "--concurrency", "9"}, {"--tenant", "x", "--batch-size", "201"}} {
		require.Error(t, runDerivativeBackfill(args))
	}
}
func TestNativeBackfillCommandDefaultsToReadOnlyDryRun(t *testing.T) {
	dsn := os.Getenv("POWERX_RUNTIME_HOST_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required for operator command acceptance")
	}
	db := host.Database(t)
	require.NoError(t, db.AutoMigrate(&m.MediaAsset{}, &m.MediaAssetVariant{}, &ef.TaskHistory{}))
	root := t.TempDir()
	tenant := uuid.NewString()
	otherTenant := uuid.NewString()
	wanted := map[string]bool{}
	manager, svc := media.BuildMediaStack(context.Background(), db, nil, media.StorageOptions{DefaultDriver: "local", Local: media.StorageLocalOptions{BasePath: root}})
	for i := 0; i < 4; i++ {
		mime := "image/png"
		if i == 2 {
			mime = "text/plain"
		}
		var imageBytes bytes.Buffer
		require.NoError(t, png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 120+i, 80))))
		raw := imageBytes.Bytes()
		if i == 2 {
			raw = []byte("plain text")
		}
		sum := sha256.Sum256(raw)
		assetTenant := tenant
		if i == 3 {
			assetTenant = otherTenant
		}
		a, err := svc.CreateAsset(context.Background(), media.CreateAssetInput{TenantUUID: assetTenant, Name: "fixture", MimeType: mime, SizeBytes: int64(len(raw)), ContentSHA256: hex.EncodeToString(sum[:]), UploadMethod: media.UploadMethodDirect})
		require.NoError(t, err)
		if i != 1 {
			_, err = manager.Put(context.Background(), "local", driver.PutObjectInput{ObjectKey: a.StorageKey, Body: bytes.NewReader(raw), Size: int64(len(raw))})
			require.NoError(t, err)
		}
		if i == 0 || i == 3 {
			wanted[a.UUID] = true
		}
	}
	cfg := config.GetDefaults()
	cfg.Deployment.Env = "test"
	cfg.EnvironmentID = "media-v2-native-command"
	cfg.SystemSnapshots.Directory = filepath.Join(t.TempDir(), "snapshots")
	cfg.Database.DSN = dsn
	cfg.Storage.DefaultDriver = "local"
	cfg.Storage.Local.BasePath = root
	cfg.Auth.JWTSecret = "isolated-command-test-secret-32bytes"
	t.Setenv("POWERX_AUTH_JWT_SECRET", cfg.Auth.JWTSecret)
	for _, key := range []string{"CORE_X_AUTH_JWT_SECRET", "CORE_X_JWT_SECRET"} {
		previous, present := os.LookupEnv(key)
		require.NoError(t, os.Unsetenv(key))
		t.Cleanup(func() {
			if present {
				_ = os.Setenv(key, previous)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "powerx.env"), []byte("# isolated operator acceptance\n"), 0600))
	raw, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	path := filepath.Join(dir, "core.yaml")
	require.NoError(t, os.WriteFile(path, raw, 0600))
	identityFile := filepath.Join(instanceidentity.CurrentDirectory(cfg.SystemSnapshots.Directory, path), "installation.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(identityFile), 0700))
	identityJSON, err := json.Marshal(map[string]string{"protocol": instanceidentity.CurrentProtocol, "id": cfg.EnvironmentID, "config_file": path})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(identityFile, identityJSON, 0600))
	_, err = config.Load(path)
	require.NoError(t, err)
	previous := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() { os.Stdout = previous; _ = r.Close(); _ = w.Close() }()
	planPath := filepath.Join(dir, "reviewed-plan.jsonl")
	runErr := runDerivativeBackfill([]string{"--config", path, "--tenant", tenant, "--batch-size", "2", "--concurrency", "2", "--report", planPath})
	_ = w.Close()
	os.Stdout = previous
	output, err := io.ReadAll(r)
	require.NoError(t, err)
	require.NoError(t, runErr, string(output))
	var report media.ImageDerivativeBackfillReport
	for _, line := range bytes.Split(output, []byte("\n")) {
		var v struct {
			Kind   string          `json:"kind"`
			Result json.RawMessage `json:"result"`
		}
		if json.Unmarshal(line, &v) == nil && v.Kind == "summary" {
			require.NoError(t, json.Unmarshal(v.Result, &report))
		}
	}
	require.True(t, report.DryRun)
	require.Equal(t, 3, report.Scanned)
	require.Equal(t, 2, report.Missing)
	require.Equal(t, 1, report.SourceMissing)
	require.Equal(t, 1, report.Skipped)
	require.Zero(t, report.Submitted)
	require.Zero(t, report.Failed)
	var count int64
	require.NoError(t, db.Model(&ef.TaskHistory{}).Count(&count).Error)
	require.Zero(t, count)
	plan, err := readDerivativePlan(planPath)
	require.NoError(t, err)
	require.Len(t, plan.Candidates, 1)
	require.Equal(t, cfg.EnvironmentID, plan.Header.EnvironmentID)
	require.Equal(t, tenant, plan.Header.TenantUUID)
	t.Logf("controlled dry-run JSONL:\n%s", output)
	if path := os.Getenv("POWERX_MEDIA_DRY_RUN_EVIDENCE"); path != "" {
		require.NoError(t, os.WriteFile(path, output, 0600))
	}
	// The explicit apply command uses the same namespace and durable history as
	// the normal worker; generation is not performed inside this command.
	mini := miniredis.RunT(t)
	cfg.Event.Fabric.RedisAddr = mini.Addr()
	cfg.Event.Fabric.RedisPassword = ""
	cfg.Event.Fabric.RedisDB = 0
	cfg.Queue.Redis.Password = ""
	cfg.Queue.Redis.DB = 0
	cfg.Cache.Password = ""
	raw, err = yaml.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0600))
	r2, w2, err := os.Pipe()
	require.NoError(t, err)
	defer r2.Close()
	defer w2.Close()
	os.Stdout = w2
	runErr = runDerivativeBackfill([]string{"--config", path, "--tenant", tenant, "--dry-run=false"})
	_ = w2.Close()
	os.Stdout = previous
	applied, err := io.ReadAll(r2)
	require.NoError(t, err)
	require.NoError(t, runErr, string(applied))
	var submitted media.ImageDerivativeBackfillReport
	for _, line := range bytes.Split(applied, []byte("\n")) {
		var v struct {
			Kind   string          `json:"kind"`
			Result json.RawMessage `json:"result"`
		}
		if json.Unmarshal(line, &v) == nil && v.Kind == "summary" {
			require.NoError(t, json.Unmarshal(v.Result, &submitted))
		}
	}
	require.False(t, submitted.DryRun)
	require.Equal(t, 1, submitted.Submitted)
	require.NoError(t, db.Model(&ef.TaskHistory{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	// The reviewed-plan path must reuse the live task and a second execution
	// must read its own journal without touching another candidate.
	for i := 0; i < 2; i++ {
		output := captureMaintenanceCommand(t, []string{"--config", path, "--plan", planPath, "--dry-run=false", "--confirm"})
		require.Contains(t, string(output), `"submission_complete":true`)
		if i == 0 {
			require.Contains(t, string(output), `"active":1`)
		} else {
			require.Contains(t, string(output), `"processed_this_run":0`)
		}
		require.NoError(t, db.Model(&ef.TaskHistory{}).Count(&count).Error)
		require.EqualValues(t, 1, count)
	}
	// A valid plan for a different installation fails before opening a journal
	// or making any database/Redis mutations.
	planBytes, err := os.ReadFile(planPath)
	require.NoError(t, err)
	wrongPlan := filepath.Join(dir, "wrong-environment.jsonl")
	require.NoError(t, os.WriteFile(wrongPlan, bytes.ReplaceAll(planBytes, []byte(cfg.EnvironmentID), []byte("another-environment")), 0600))
	err = runDerivativeBackfill([]string{"--config", path, "--plan", wrongPlan, "--dry-run=false", "--confirm"})
	require.ErrorContains(t, err, "MEDIA_BACKFILL_PLAN_ENVIRONMENT_MISMATCH")
	_, err = os.Stat(wrongPlan + ".progress.jsonl")
	require.True(t, os.IsNotExist(err))
	// No tenant argument: scan every tenant and execute the exact global plan.
	globalPath := filepath.Join(dir, "all-tenants.jsonl")
	captureMaintenanceCommand(t, []string{"--config", path, "--report", globalPath, "--batch-size", "2"})
	globalPlan, err := readDerivativePlan(globalPath)
	require.NoError(t, err)
	require.Equal(t, derivativeScopeAll, globalPlan.Header.Scope)
	require.Empty(t, globalPlan.Header.TenantUUID)
	require.Zero(t, globalPlan.Header.Limit)
	require.Equal(t, 4, globalPlan.Summary.Scanned)
	require.Equal(t, 2, globalPlan.Summary.TenantCount)
	require.Len(t, globalPlan.Candidates, 2)
	require.NoError(t, db.Model(&ef.TaskHistory{}).Count(&count).Error)
	require.EqualValues(t, 1, count, "global dry-run does not write task history")
	outputGlobal := captureMaintenanceCommand(t, []string{"--config", path, "--plan", globalPath, "--dry-run=false", "--confirm", "--batch-size", "1"})
	require.Contains(t, string(outputGlobal), `"scope":"all_tenants"`)
	require.Contains(t, string(outputGlobal), `"submission_complete":true`)
	require.Contains(t, string(outputGlobal), `"submitted":1`)
	require.Contains(t, string(outputGlobal), `"active":1`)
	require.NoError(t, db.Model(&ef.TaskHistory{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
	var histories []ef.TaskHistory
	require.NoError(t, db.Find(&histories).Error)
	var historyTenants []string
	for _, history := range histories {
		var msg event_bus.TaskMessage
		require.NoError(t, json.Unmarshal([]byte(history.Payload), &msg))
		var task struct {
			Tenant string `json:"tenant_uuid"`
		}
		require.NoError(t, json.Unmarshal(msg.Payload, &task))
		historyTenants = append(historyTenants, task.Tenant)
	}
	require.ElementsMatch(t, []string{tenant, otherTenant}, historyTenants)
	outputGlobal = captureMaintenanceCommand(t, []string{"--config", path, "--plan", globalPath, "--dry-run=false", "--confirm"})
	require.Contains(t, string(outputGlobal), `"processed_this_run":0`)
	require.NoError(t, db.Model(&ef.TaskHistory{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
	namespace, err := cfg.RedisNamespace()
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer client.Close()
	base, err := event_bus.NewRedisStreamTaskDriver(client, 2*time.Minute, namespace)
	require.NoError(t, err)
	svc.ConfigureDerivativeQueue(taskqueue.NewStreamDriver(db, base))
	ready := make(chan struct{})
	var once sync.Once
	var readyMu sync.Mutex
	readyAssets := map[string]bool{}
	svc.SetDerivativeEvents(func(ctx context.Context, tenant, asset string) {
		state, err := svc.ImageDerivativeStatus(ctx, tenant, asset)
		if err == nil && state.Status == "ready" {
			readyMu.Lock()
			defer readyMu.Unlock()
			if wanted[asset] {
				readyAssets[asset] = true
			}
			if len(readyAssets) == len(wanted) {
				once.Do(func() { close(ready) })
			}
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- svc.RunImageDerivativeWorker(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(6 * time.Second):
			t.Error("controlled backfill worker did not stop")
		}
	}()
	select {
	case <-ready:
	case <-time.After(15 * time.Second):
		t.Fatal("both tenants' submitted backfills did not become ready")
	}
	t.Logf("controlled explicit apply JSONL:\n%s", applied)
	if path := os.Getenv("POWERX_MEDIA_APPLY_EVIDENCE"); path != "" {
		require.NoError(t, os.WriteFile(path, applied, 0600))
	}

}

func captureMaintenanceCommand(t *testing.T, args []string) []byte {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	require.NoError(t, err)
	previous := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = previous; _ = f.Close() }()
	err = runDerivativeBackfill(args)
	os.Stdout = previous
	require.NoError(t, err)
	raw, err := os.ReadFile(f.Name())
	require.NoError(t, err)
	return raw
}
