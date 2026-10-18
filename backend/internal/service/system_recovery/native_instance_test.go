package system_recovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Actual PG/Core/Web/media/config restore on an owned fixed installation. Source
// development services are never used as the destructive test target.
func exerciseNativeInstance(t *testing.T, ctx context.Context, root string, p Profile, d Deployment, directory, snapshotA string) {
	t.Helper()
	p.Local, p.InPlace, p.ExclusiveWriters, p.DedicatedRedis = true, true, true, true
	p.SnapshotDirectory = directory
	p.CandidateDirectory = filepath.Dir(d.Root)
	p.Initial = d
	p.Initial.ConfigFile = filepath.Join(d.Root, "etc/config.yaml")
	p.File = filepath.Join(root, "instance-profile.json")
	p.SourceRedisRDB = filepath.Join(d.Root, "redis/dump.rdb")
	p.Targets = []snapshots.RecoveryTarget{{ID: "current", Mode: "formal", Environment: "dev", AllowOverwrite: true, AllowOnlineData: true}}
	raw, err := os.ReadFile(d.ConfigFile)
	require.NoError(t, err)
	var cfg config.Config
	require.NoError(t, yaml.Unmarshal(raw, &cfg))
	issuerA := cfg.Auth.Issuer
	cfg.Recovery.ProfileFile = p.File
	require.NoError(t, writeInstallationConfig(p.Initial.ConfigFile, &cfg))
	d = p.Initial
	require.NoError(t, p.Validate())
	require.NoError(t, snapshots.WriteCoordinatorJSON(p.File, p))
	store, err := snapshots.NewCoordinatorStore(p.Directory, p.Targets)
	require.NoError(t, err)
	service := snapshots.NewService(ctx, snapshots.Config{Directory: directory}, snapshots.Options{Coordinator: true})
	defer service.Close()
	n := &Native{Profile: p, Store: store, SnapshotStore: service}
	c := &Coordinator{Store: store, Driver: n, Initial: d}
	_, err = c.Active()
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		require.NoError(t, StopCandidateProcesses(cleanup, d.Root))
	})
	// B is distinct in the database, media and the official config.
	cfg.Auth.Issuer = "owned-instance-state-B"
	require.NoError(t, writeInstallationConfig(d.ConfigFile, &cfg))
	require.NoError(t, n.Start(ctx, d, false))
	_, err = n.Verify(ctx, d)
	require.NoError(t, err)
	db, err := pgx.Connect(ctx, cfg.Database.DSN)
	require.NoError(t, err)
	_, err = db.Exec(ctx, "INSERT INTO public.recovery_fixture_orders(label) VALUES ('test5')")
	require.NoError(t, err)
	b := []byte("instance-media-B")
	sum := sha256.Sum256(b)
	require.NoError(t, os.WriteFile(filepath.Join(d.Root, "media/fixture/sample.txt"), b, 0600))
	_, err = db.Exec(ctx, "UPDATE public.media_assets SET expected_checksum=$1,size_bytes=$2", hex.EncodeToString(sum[:]), len(b))
	require.NoError(t, err)
	db.Close(ctx)
	if os.Getenv("POWERX_RECOVERY_INSTANCE_CRASH") == "1" {
		crashed, err := store.EnqueueInstanceRestore(privileged(ctx), snapshotA, uuid.NewString(), 1)
		require.NoError(t, err)
		crashOwnedCoordinator(t, ctx, root, p)
		require.NoError(t, c.Recover(ctx))
		crashed, err = store.Get(crashed.ID)
		require.NoError(t, err)
		require.Equal(t, "rolled_back", crashed.Status, crashed.Error)
		db, err = pgx.Connect(ctx, cfg.Database.DSN)
		require.NoError(t, err)
		var count int
		require.NoError(t, db.QueryRow(ctx, "SELECT count(*) FROM public.recovery_fixture_orders WHERE label='test5'").Scan(&count))
		db.Close(ctx)
		require.Equal(t, 1, count)
	}
	var restore snapshots.ManagedJob
	if os.Getenv("POWERX_RECOVERY_PORTAL_TEST") == "1" {
		restore = exerciseOfflineNativePortal(t, ctx, c, n, d, snapshotA)
	} else {
		restore, err = store.EnqueueInstanceRestore(privileged(ctx), snapshotA, uuid.NewString(), 1)
		require.NoError(t, err)
		require.NoError(t, c.RunOnce(ctx))
	}
	restore, err = store.Get(restore.ID)
	require.NoError(t, err)
	require.Equal(t, "complete", restore.Status, restore.Error)
	require.NotEmpty(t, restore.RollbackSnapshotID)
	assertState := func(want int, issuer, media string) {
		t.Helper()
		db, err := pgx.Connect(ctx, cfg.Database.DSN)
		require.NoError(t, err)
		defer db.Close(ctx)
		var count int
		require.NoError(t, db.QueryRow(ctx, "SELECT count(*) FROM public.recovery_fixture_orders WHERE label='test5'").Scan(&count))
		require.Equal(t, want, count)
		raw, err := os.ReadFile(d.ConfigFile)
		require.NoError(t, err)
		var got config.Config
		require.NoError(t, yaml.Unmarshal(raw, &got))
		require.Equal(t, issuer, got.Auth.Issuer)
		require.Equal(t, cfg.Database.DSN, got.Database.DSN)
		data, err := os.ReadFile(filepath.Join(d.Root, "media/fixture/sample.txt"))
		require.NoError(t, err)
		require.Equal(t, media, string(data))
	}
	assertState(0, issuerA, "media-at-A")
	a, err := c.Active()
	require.NoError(t, err)
	require.Equal(t, d.ID, a.Deployment.ID)
	require.Equal(t, d.ConfigFile, a.Deployment.ConfigFile)
	require.Empty(t, a.LastSwitchID)
	_, err = os.Stat(filepath.Join(p.CandidateDirectory, "retained-"+restore.ID))
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(p.CandidateDirectory, restore.ID))
	require.True(t, os.IsNotExist(err))
	// A full shutdown and a fresh coordinator read the same fixed config and PG.
	require.NoError(t, StopCandidateProcesses(ctx, d.Root))
	p, err = ReadProfile(p.File)
	require.NoError(t, err)
	restarted := &Native{Profile: p, Store: store, SnapshotStore: service}
	require.NoError(t, restarted.EnsureActive(ctx, a))
	_, err = restarted.Verify(ctx, d)
	require.NoError(t, err)
	assertState(0, issuerA, "media-at-A")
	// Return to B by restoring its protection snapshot, never an environment switch.
	back, err := store.EnqueueInstanceRestore(privileged(ctx), restore.RollbackSnapshotID, uuid.NewString(), a.Generation)
	require.NoError(t, err)
	c.Driver = restarted
	require.NoError(t, c.RunOnce(ctx))
	back, err = store.Get(back.ID)
	require.NoError(t, err)
	require.Equal(t, "complete", back.Status, back.Error)
	assertState(1, "owned-instance-state-B", "instance-media-B")
	// A clean second server imports A without touching the first instance B.
	p2 := p
	p2.InstanceID = uuid.NewString()
	p2.Targets = []snapshots.RecoveryTarget{{ID: "current", Mode: "isolated", Environment: "dev", AllowOverwrite: true}}
	p2.Initial.ID = uuid.NewString()
	p2.Initial.Root = filepath.Join(p.CandidateDirectory, p2.Initial.ID)
	p2.Initial.ConfigFile = filepath.Join(p2.Initial.Root, "etc/config.yaml")
	p2.Initial.CoreBinary = filepath.Join(p2.Initial.Root, "programs/core/powerx")
	p2.Initial.WebOutput = filepath.Join(p2.Initial.Root, "programs/core/web-admin")
	p2.Initial.PostgresData = filepath.Join(p2.Initial.Root, "postgres")
	p2.Initial.SocketDirectory = filepath.Join(p2.Initial.Root, "s")
	for _, port := range []*int{&p2.Initial.CorePort, &p2.Initial.GRPCPort, &p2.Initial.WebPort, &p2.Initial.RedisPort, &p2.Initial.PostgresPort} {
		*port, err = freePort()
		require.NoError(t, err)
	}
	p2.Initial.PublicBaseURL = fmt.Sprintf("http://127.0.0.1:%d", p2.Initial.WebPort)
	p2.Directory = filepath.Join(root, "second-ops")
	p2.File = filepath.Join(root, "second-profile.json")
	p2.SourceRedisRDB = filepath.Join(p2.Initial.Root, "redis/dump.rdb")
	require.NoError(t, snapshots.WriteCoordinatorJSON(p2.File, p2))
	require.NoError(t, InitializeInstallation(ctx, p2.File, snapshotA, snapshotA))
	require.Error(t, InitializeInstallation(ctx, p2.File, snapshotA, snapshotA), "initialization cannot overwrite an existing installation")
	store2, err := snapshots.NewCoordinatorStore(p2.Directory, p2.Targets)
	require.NoError(t, err)
	n2 := &Native{Profile: p2, Store: store2, SnapshotStore: service}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		require.NoError(t, StopCandidateProcesses(cleanup, p2.Initial.Root))
	})
	c2 := &Coordinator{Store: store2, Initial: p2.Initial}
	a2, err := c2.Active()
	require.NoError(t, err)
	require.NoError(t, n2.EnsureActive(ctx, a2))
	_, err = n2.Verify(ctx, p2.Initial)
	require.NoError(t, err)
	raw, err = os.ReadFile(p2.Initial.ConfigFile)
	require.NoError(t, err)
	var second config.Config
	require.NoError(t, yaml.Unmarshal(raw, &second))
	require.NotEqual(t, cfg.Database.DSN, second.Database.DSN)
	db2, err := pgx.Connect(ctx, second.Database.DSN)
	require.NoError(t, err)
	var secondCount int
	require.NoError(t, db2.QueryRow(ctx, "SELECT count(*) FROM public.recovery_fixture_orders WHERE label='test5'").Scan(&secondCount))
	db2.Close(ctx)
	require.Zero(t, secondCount)
	assertState(1, "owned-instance-state-B", "instance-media-B")
	require.NoError(t, StopCandidateProcesses(ctx, p2.Initial.Root))
	t.Logf("fixed instance restored A, restarted with A, then restored protection B; config path and DSN unchanged; test5 restored; temporary environments removed")
}
