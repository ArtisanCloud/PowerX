package plugininstall

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/task_queue"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	ef "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_install"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const testPlugin = "com.powerx.plugins.install-test"

type installerStub struct {
	mu          sync.Mutex
	p           pm.Plugin
	calls       int
	enableCalls int
	hook        func(context.Context, pm.InstallOptions) (pm.Plugin, error)
	got         pm.InstallOptions
}

func (f *installerStub) InstallFromFile(ctx context.Context, path string, o pm.InstallOptions) (pm.Plugin, error) {
	f.mu.Lock()
	f.calls++
	f.got = o
	hook := f.hook
	f.mu.Unlock()
	if hook != nil {
		return hook(ctx, o)
	}
	p := pm.Plugin{ID: testPlugin, Version: "1.0.0", Revision: o.InstallationRevision}
	if e := o.Progress(ctx, "migrating", p); e != nil {
		return p, e
	}
	f.mu.Lock()
	f.p = p
	f.mu.Unlock()
	return p, nil
}
func (f *installerStub) InstallFromURL(ctx context.Context, u, sha, sig string, o pm.InstallOptions) (pm.Plugin, error) {
	return f.InstallFromFile(ctx, "", o)
}
func (f *installerStub) Get(ctx context.Context, id string) (pm.Plugin, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.p.ID == "" {
		return pm.Plugin{}, pm.NewError(pm.CodeNotFound)
	}
	return f.p, nil
}
func (f *installerStub) Enable(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enableCalls++
	return ctx.Err()
}

type fixture struct {
	s             *Service
	db            *gorm.DB
	mgr           *installerStub
	ctx           context.Context
	tenant, actor string
	revoked       atomic.Bool
	finalize      atomic.Int32
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("POWERX_INSTALL_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL and Redis required: POWERX_INSTALL_TEST_DSN / POWERX_INSTALL_TEST_REDIS")
	}
	u, e := url.Parse(dsn)
	require.NoError(t, e)
	require.True(t, strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), "powerx_install_test_"))
	addr := os.Getenv("POWERX_INSTALL_TEST_REDIS")
	require.NotEmpty(t, addr)
	db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, e)
	old := core.PowerXSchema
	core.PowerXSchema = "install_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	schema := core.PowerXSchema
	require.NoError(t, db.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	require.NoError(t, db.AutoMigrate(&m.Installation{}, &ef.TaskHistory{}))
	t.Cleanup(func() {
		require.NoError(t, db.Exec(`DROP SCHEMA "`+schema+`" CASCADE`).Error)
		core.PowerXSchema = old
		pool, _ := db.DB()
		pool.Close()
	})
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { client.Close() })
	base, e := event_bus.NewRedisStreamTaskDriver(client, time.Second)
	require.NoError(t, e)
	f := &fixture{db: db, mgr: &installerStub{}, tenant: uuid.NewString(), actor: uuid.NewString()}
	f.ctx = reqctx.WithTenantUUID(context.Background(), f.tenant)
	f.ctx = reqctx.WithUserID(f.ctx, 1)
	f.ctx = reqctx.WithIsRoot(f.ctx, true)
	f.ctx = reqctx.WithClaims(f.ctx, &reqctx.CoreXClaims{UserID: 1, UserUUID: f.actor, IsRoot: true, TenantUUID: f.tenant})
	f.s, e = NewService(db, Options{Root: filepath.Join(t.TempDir(), "staging"), InstanceID: schema, Secret: []byte(strings.Repeat("x", 32)), Queue: taskqueue.NewStreamDriverWithRetryDelay(db, base, time.Millisecond), Manager: f.mgr, Authorize: func(ctx context.Context, tenant string, actor uint64) error {
		if f.revoked.Load() {
			return problem(403, "REVOKED")
		}
		return nil
	}, Finalize: func(ctx context.Context, tenant string, p pm.Plugin) error { f.finalize.Add(1); return nil }})
	require.NoError(t, e)
	return f
}
func (f *fixture) create(t *testing.T) View {
	t.Helper()
	v, e := f.s.Create(f.ctx, Input{RequestID: uuid.NewString(), SourceKind: "local", PluginID: testPlugin, Version: "1.0.0", Enable: true})
	require.NoError(t, e)
	return v
}
func (f *fixture) upload(t *testing.T, v View) View {
	t.Helper()
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "plugin.yaml"), []byte("id: "+testPlugin+"\nversion: 1.0.0\n"), 0600))
	out, e := f.s.AcceptSource(f.ctx, v.TaskID, src)
	require.NoError(t, e)
	return out
}
func (f *fixture) delivery(t *testing.T) event_bus.StreamTaskDelivery {
	t.Helper()
	var d []event_bus.StreamTaskDelivery
	require.Eventually(t, func() bool {
		var e error
		d, e = f.s.opts.Queue.Dequeue(context.Background(), queueTenant, f.s.subscriber, "plugin-install-workers", uuid.NewString(), 1, time.Millisecond)
		return e == nil && len(d) > 0
	}, 8*time.Second, 20*time.Millisecond)
	return d[0]
}
func TestAcceptedTaskSurvivesRequestCancellation(t *testing.T) {
	f := newFixture(t)
	request, cancel := context.WithCancel(f.ctx)
	v, e := f.s.Create(request, Input{RequestID: uuid.NewString(), SourceKind: "local", PluginID: testPlugin, Version: "1.0.0", Enable: true, Force: true, ExpectedRevision: "baseline", ConfirmLegacyProvider: true})
	require.NoError(t, e)
	v = f.upload(t, v)
	cancel()
	require.NoError(t, f.s.handleDelivery(context.Background(), f.delivery(t)))
	out, e := f.s.Get(f.ctx, v.TaskID)
	require.NoError(t, e)
	require.Equal(t, "succeeded", out.Status)
	require.Equal(t, 1, f.mgr.calls)
	require.Equal(t, 1, f.mgr.enableCalls)
	require.EqualValues(t, 1, f.finalize.Load())
	require.True(t, f.mgr.got.Force)
	require.True(t, f.mgr.got.ConfirmLegacyProvider)
	require.Equal(t, "baseline", f.mgr.got.ExpectedRevision)
	require.Equal(t, v.TaskID, f.mgr.got.InstallationRevision)
	require.True(t, f.mgr.got.AutoEnable)
	require.True(t, f.mgr.got.RequireAbsent)
	_, e = os.Stat(filepath.Join(f.s.taskDirectory(v.TaskID), "source"))
	require.True(t, os.IsNotExist(e))
	var count int64
	require.NoError(t, f.db.Model(&ef.TaskHistory{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
func TestIntentIdempotencyEncryptionAndOwnership(t *testing.T) {
	f := newFixture(t)
	in := Input{RequestID: uuid.NewString(), SourceKind: "url", URL: "https://example.invalid/package.tgz?signature=do-not-expose", Enable: true}
	a, e := f.s.Create(f.ctx, in)
	require.NoError(t, e)
	b, e := f.s.Create(f.ctx, in)
	require.NoError(t, e)
	require.Equal(t, a.TaskID, b.TaskID)
	row, e := f.s.repo.Get(f.ctx, f.tenant, a.TaskID)
	require.NoError(t, e)
	require.NotContains(t, string(row.PrivateRequest), "do-not-expose")
	input, e := f.s.input(row)
	require.NoError(t, e)
	require.Equal(t, in.URL, input.URL)
	other := reqctx.WithTenantUUID(f.ctx, uuid.NewString())
	_, e = f.s.Get(other, a.TaskID)
	require.Equal(t, 404, dto.StatusCode(e))
	other = reqctx.WithClaims(f.ctx, &reqctx.CoreXClaims{UserID: 2, UserUUID: uuid.NewString(), IsRoot: true})
	_, e = f.s.Get(other, a.TaskID)
	require.Equal(t, 404, dto.StatusCode(e))
	in.Enable = false
	_, e = f.s.Create(f.ctx, in)
	require.Equal(t, "PLUGIN_INSTALL_REQUEST_CONFLICT", dto.CodeOf(e))
	row.TenantUUID = uuid.NewString()
	_, e = f.s.input(row)
	require.Error(t, e)
}
func TestWorkerRevocationStopsBeforeMutation(t *testing.T) {
	f := newFixture(t)
	v := f.upload(t, f.create(t))
	f.revoked.Store(true)
	require.NoError(t, f.s.handleDelivery(context.Background(), f.delivery(t)))
	f.revoked.Store(false)
	out, e := f.s.Get(f.ctx, v.TaskID)
	require.NoError(t, e)
	require.Equal(t, "PLUGIN_INSTALL_ACTOR_REVOKED", out.ErrorCode)
	require.Zero(t, f.mgr.calls)
}
func TestCrashReconcilesCommitAndNeverReplaysMigration(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprint(committed), func(t *testing.T) {
			f := newFixture(t)
			v := f.upload(t, f.create(t))
			d := f.delivery(t)
			run, cancel := context.WithCancel(context.Background())
			f.mgr.hook = func(ctx context.Context, o pm.InstallOptions) (pm.Plugin, error) {
				p := pm.Plugin{ID: testPlugin, Version: "1.0.0", Revision: o.InstallationRevision}
				require.NoError(t, o.Progress(ctx, "migrating", p))
				if committed {
					f.mgr.mu.Lock()
					f.mgr.p = p
					f.mgr.mu.Unlock()
				}
				cancel()
				return pm.Plugin{}, ctx.Err()
			}
			require.ErrorIs(t, f.s.handleDelivery(run, d), context.Canceled)
			time.Sleep(1200 * time.Millisecond)
			fresh, e := NewService(f.db, f.s.opts)
			require.NoError(t, e)
			f.s = fresh
			require.NoError(t, f.s.opts.Queue.Recover(context.Background(), queueTenant, f.s.subscriber))
			require.NoError(t, f.s.handleDelivery(context.Background(), f.delivery(t)))
			out, e := f.s.Get(f.ctx, v.TaskID)
			require.NoError(t, e)
			require.Equal(t, 1, f.mgr.calls)
			if committed {
				require.Equal(t, "succeeded", out.Status)
				require.Equal(t, 1, f.mgr.enableCalls)
			} else {
				require.Equal(t, "needs_attention", out.Status)
				require.Equal(t, "PLUGIN_INSTALL_INTERRUPTED_REVIEW_REQUIRED", out.ErrorCode)
				require.Zero(t, f.mgr.enableCalls)
			}
		})
	}
}
func TestSourceChangedAndDiscardSafety(t *testing.T) {
	f := newFixture(t)
	v := f.create(t)
	cancelled, e := f.s.Discard(f.ctx, v.TaskID)
	require.NoError(t, e)
	require.Equal(t, "cancelled", cancelled.Status)
	v = f.upload(t, f.create(t))
	_, e = f.s.Discard(f.ctx, v.TaskID)
	require.Equal(t, "PLUGIN_INSTALL_TASK_ACTIVE", dto.CodeOf(e))
	require.NoError(t, os.WriteFile(filepath.Join(f.s.taskDirectory(v.TaskID), "source", "plugin.yaml"), []byte("changed"), 0600))
	require.NoError(t, f.s.handleDelivery(context.Background(), f.delivery(t)))
	out, e := f.s.Get(f.ctx, v.TaskID)
	require.NoError(t, e)
	require.Equal(t, "PLUGIN_INSTALL_SOURCE_CHANGED", out.ErrorCode)
	require.Zero(t, f.mgr.calls)
}
func TestReplacementFailureReceiptIsSafeAndComplete(t *testing.T) {
	f := newFixture(t)
	v := f.upload(t, f.create(t))
	f.mgr.hook = func(context.Context, pm.InstallOptions) (pm.Plugin, error) {
		return pm.Plugin{}, &pm.ReplacementRequiredError{PluginID: testPlugin, RequiresProviderConfirmation: true, Current: pm.InstallPreview{Version: "old", Revision: "old-revision", Origin: pm.InstallOrigin{SourceID: "secret-url"}}, Target: pm.InstallPreview{Version: "1.0.0"}}
	}
	require.NoError(t, f.s.handleDelivery(context.Background(), f.delivery(t)))
	out, e := f.s.Get(f.ctx, v.TaskID)
	require.NoError(t, e)
	require.Equal(t, "PLUGIN_REPLACEMENT_REQUIRED", out.ErrorCode)
	require.Contains(t, string(out.FailureDetails), "old-revision")
	require.NotContains(t, string(out.FailureDetails), "secret-url")
}
func TestDisabledWorkerAndMissingSchemaRejectAcceptance(t *testing.T) {
	f := newFixture(t)
	f.s.opts.Disabled = true
	_, e := f.s.Create(f.ctx, Input{})
	require.Equal(t, "PLUGIN_INSTALL_WORKER_DISABLED", dto.CodeOf(e))
	f.s.opts.Disabled = false
	require.NoError(t, f.db.Migrator().DropTable(&m.Installation{}))
	_, e = f.s.Create(f.ctx, Input{})
	require.Equal(t, "PLUGIN_INSTALL_SCHEMA_NOT_READY", dto.CodeOf(e))
}
func TestSourceRejectsSymlinksAndHardlinks(t *testing.T) {
	for _, hard := range []bool{false, true} {
		t.Run(fmt.Sprint(hard), func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "source"), []byte("private"), 0600))
			var e error
			if hard {
				e = os.Link(filepath.Join(root, "source"), filepath.Join(root, "linked"))
			} else {
				e = os.Symlink(filepath.Join(root, "source"), filepath.Join(root, "linked"))
			}
			require.NoError(t, e)
			_, _, e = copySource(context.Background(), root, "")
			require.Equal(t, "PLUGIN_INSTALL_SOURCE_LINK_FORBIDDEN", dto.CodeOf(e))
		})
	}
}

func TestDiscardRemovesCrashUploadTreesButKeepsReceipt(t *testing.T) {
	f := newFixture(t)
	v := f.create(t)
	dir := f.s.taskDirectory(v.TaskID)
	orphan := filepath.Join(dir, "staging-"+uuid.NewString())
	require.NoError(t, os.MkdirAll(orphan, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(orphan, "partial"), []byte("partial upload"), 0600))
	_, e := f.s.Discard(f.ctx, v.TaskID)
	require.NoError(t, e)
	_, e = os.Stat(orphan)
	require.True(t, os.IsNotExist(e))
	out, e := f.s.Get(f.ctx, v.TaskID)
	require.NoError(t, e)
	require.Equal(t, "cancelled", out.Status)
}

func TestAcceptanceRequiresRunningProductionWorker(t *testing.T) {
	f := newFixture(t)
	f.s.opts.RequireWorker = true
	_, e := f.s.Create(f.ctx, Input{})
	require.Equal(t, "PLUGIN_INSTALL_WORKER_DISABLED", dto.CodeOf(e))
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = f.s.Run(ctx) }()
	defer func() { stop(); <-done }()
	require.Eventually(t, func() bool { return f.s.running.Load() }, 3*time.Second, 10*time.Millisecond)
	v := f.create(t)
	require.Equal(t, "awaiting_upload", v.Status)
}

func TestURLPendingActivationKeepsTypedRecoveryTarget(t *testing.T) {
	f := newFixture(t)
	v, e := f.s.Create(f.ctx, Input{RequestID: uuid.NewString(), SourceKind: "url", URL: "https://example.invalid/plugin.tgz", Enable: true})
	require.NoError(t, e)
	f.mgr.hook = func(ctx context.Context, o pm.InstallOptions) (pm.Plugin, error) {
		p := pm.Plugin{ID: testPlugin, Version: "1.0.0", Revision: o.InstallationRevision}
		return pm.Plugin{}, pm.PendingInstallation(p, "runtime_activation", errors.New("health failed"))
	}
	require.NoError(t, f.s.handleDelivery(context.Background(), f.delivery(t)))
	out, e := f.s.Get(f.ctx, v.TaskID)
	require.NoError(t, e)
	require.Equal(t, "needs_attention", out.Status)
	require.Equal(t, testPlugin, out.PluginID)
	require.Equal(t, v.TaskID, out.InstalledRevision)
}
