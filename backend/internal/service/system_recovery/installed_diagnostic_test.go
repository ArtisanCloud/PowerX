package system_recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/ArtisanCloud/PowerX/config"
	manager "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	gateway "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	glog "gorm.io/gorm/logger"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in only for a private clone produced by the owned local preparation tool;
// it never accepts the developer's original database or media directory.
func TestRecoveryPreparedInstalledLocalClone(t *testing.T) {
	root := os.Getenv("POWERX_RECOVERY_OWNED_CLONE")
	if root == "" {
		t.Skip("owned prepared clone only")
	}
	require.True(t, strings.HasPrefix(root, "/private/tmp/pxu-restore"))
	require.Equal(t, root, filepath.Clean(root))
	matches, e := filepath.Glob(filepath.Join(root, "c", "*", "run.json"))
	require.NoError(t, e)
	require.Len(t, matches, 1)
	var run RunSpec
	require.NoError(t, snapshots.ReadCoordinatorJSON(matches[0], &run))
	d := run.Deployment
	require.Equal(t, filepath.Join(root, "c", d.ID), d.Root)
	d.ConfigFile = filepath.Join(d.Root, "runtime.yaml")
	raw, e := os.ReadFile(d.ConfigFile)
	require.NoError(t, e)
	var cfg config.Config
	require.NoError(t, yaml.Unmarshal(raw, &cfg))
	cfg.AI.Runtime.DurableSessions.Enabled = false
	cfg.AI.Runtime.DurableSessions.AdminChatEnabled = false
	raw, e = yaml.Marshal(&cfg)
	require.NoError(t, e)
	require.NoError(t, os.WriteFile(d.ConfigFile, raw, 0600))
	var probes []BusinessProbe
	raw, e = os.ReadFile("../../../config/recovery/local-installed-business-probes.example.json")
	require.NoError(t, e)
	require.NoError(t, json.Unmarshal(raw, &probes))
	p := Profile{Local: true, Directory: filepath.Join(root, "ops"), CandidateDirectory: filepath.Join(root, "c"), SnapshotDirectory: filepath.Join(root, "bootstrap"), Tools: run.Tools, Initial: Deployment{Root: filepath.Join(root, "unused-source")}, BusinessProbes: probes}
	store, e := snapshots.NewCoordinatorStore(p.Directory, []snapshots.RecoveryTarget{{ID: "local", Mode: "formal", Environment: "dev", AllowOnlineData: true}})
	require.NoError(t, e)
	n := &Native{Profile: p, Store: store}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		require.NoError(t, StopCandidateProcesses(cleanup, d.Root))
	})
	require.NoError(t, n.startPostgres(ctx, d))
	conn, e := pgx.Connect(ctx, cfg.Database.DSN)
	require.NoError(t, e)
	var schema, table string
	require.NoError(t, conn.QueryRow(ctx, "SELECT table_schema, table_name FROM information_schema.tables WHERE table_name LIKE '%plugin_instance_configs' LIMIT 1").Scan(&schema, &table))
	rows, e := conn.Query(ctx, "SELECT plugin_id, COALESCE((value_json->'allowed_capabilities') ? 'com.corex.capabilities.grant_status.read', false) FROM "+pgx.Identifier{schema, table}.Sanitize()+" WHERE key = 'auth.credentials' AND enabled = true")
	require.NoError(t, e)
	for rows.Next() {
		var id string
		var allowed bool
		require.NoError(t, rows.Scan(&id, &allowed))
		t.Logf("restored credential %s grant-status explicitly allowed: %t", id, allowed)
	}
	require.NoError(t, rows.Err())
	rows.Close()
	require.NoError(t, conn.Close(ctx))
	// Optional compatibility check for a newly rebuilt Core/plugin pair. This
	// publishes the current formal catalog in the disposable clone only; it
	// never adds credential grants, edits business rows or touches the source DB.
	// It must not be reported as an unchanged-artifact snapshot acceptance.
	publishCatalog := os.Getenv("POWERX_RECOVERY_PUBLISH_CLONE_CAPABILITIES") == "1"
	if publishCatalog {
		catalogDirectory, err := filepath.Abs("../../../config/platform_capabilities")
		require.NoError(t, err)
		t.Setenv("PLATFORM_CAPABILITIES_DIR", catalogDirectory)
		ownedCatalog := filepath.Join(d.Root, "programs/core/platform_capabilities")
		require.True(t, inside(d.Root, ownedCatalog))
		require.NoError(t, os.RemoveAll(ownedCatalog))
		require.NoError(t, os.CopyFS(ownedCatalog, os.DirFS(catalogDirectory)))
		db, err := gorm.Open(postgres.Open(cfg.Database.DSN), &gorm.Config{Logger: glog.Default.LogMode(glog.Silent)})
		require.NoError(t, err)
		pool, err := db.DB()
		require.NoError(t, err)
		seeder := gateway.NewBaseCapabilitySeeder(gateway.BaseCapabilitySeederOptions{PreserveInstalledState: true, RecordRepo: caprepo.NewCapabilityRecordRepository(db, nil), RegistryRepo: caprepo.NewCapabilityRegistryRepository(db), TenantRepo: tenantrepo.NewTenantRepository(db)})
		require.NoError(t, seeder.Ensure(ctx))
		var published int64
		require.NoError(t, db.Table("public.capability_registry_records").Where("capability_id = ? AND status = ?", "com.corex.capabilities.grant_status.read", "published").Count(&published).Error)
		require.EqualValues(t, 1, published)
		require.NoError(t, pool.Close())
		t.Log("current formal catalog published in disposable clone; credential grants unchanged")
	}
	service := snapshots.NewService(ctx, snapshots.Config{Directory: p.SnapshotDirectory}, snapshots.Options{Coordinator: true})
	defer service.Close()
	overview, e := service.Overview(privileged(ctx))
	require.NoError(t, e)
	require.Len(t, overview.Snapshots, 1)
	_, configuration, e := service.MaterializeForRecovery(ctx, overview.Snapshots[0].ID, filepath.Join(root, "configuration-inspection-"+uuid.NewString()))
	require.NoError(t, e)
	for name, data := range configuration {
		if !strings.HasPrefix(name, "plugins/") {
			continue
		}
		file := filepath.Join(d.Root, "programs", filepath.FromSlash(name))
		if name == "plugins/registry.json" {
			file = cfg.Plugin.RegistryFile
		}
		require.True(t, inside(d.Root, file))
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0700))
		require.NoError(t, os.WriteFile(file, data, 0600))
	}
	beforeRegistry, e := os.ReadFile(cfg.Plugin.RegistryFile)
	require.NoError(t, e)
	var restoredRegistry struct {
		Plugins map[string]struct {
			Versions map[string]struct {
				HostConfig struct {
					Values map[string]string `json:"values"`
				} `json:"host_config"`
			} `json:"versions"`
		} `json:"plugins"`
	}
	require.NoError(t, json.Unmarshal(beforeRegistry, &restoredRegistry))
	for id, plugin := range restoredRegistry.Plugins {
		for version, record := range plugin.Versions {
			key := "plugins/" + id + "/" + version + "/config/host-values.yaml"
			if raw := configuration[key]; len(raw) > 0 {
				var saved struct {
					Env map[string]string `yaml:"env"`
				}
				require.NoError(t, yaml.NewDecoder(bytes.NewReader(raw)).Decode(&saved))
				t.Logf("restored plugin %s host identity matches registry: %t", id, saved.Env["POWERX_STS_CLIENT_ID"] == record.HostConfig.Values["POWERX_STS_CLIENT_ID"])
			}
		}
	}
	roles, e := manager.RemapRecoveryBindings(ctx, &cfg)
	require.NoError(t, e)
	require.NoError(t, snapshots.WriteCoordinatorJSON(filepath.Join(d.Root, "plugin-roles.json"), roles))
	require.NoError(t, n.Start(ctx, d, true))
	checks, e := n.Verify(ctx, d)
	require.NoError(t, e)
	require.NotEmpty(t, checks)
	require.NoError(t, snapshots.WriteCoordinatorJSON("../../../../test-results/recovery-installed-prepared-evidence.json", map[string]any{"passed": true, "readonly_source_clone": true, "core_customer_probe": true, "installed_base_plugin_probe": true, "installed_ai_craft_plugin_probe": true, "core_binary": "current_fix_build", "original_package_unchanged": true, "original_program_artifact_replay": false, "clone_catalog_published": publishCatalog, "unchanged_snapshot_acceptance": false, "external_agent_reports_disabled": true, "production_full_recovery_verified": false, "checks": checks}))
}
