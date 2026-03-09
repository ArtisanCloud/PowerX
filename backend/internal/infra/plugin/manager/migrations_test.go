package manager

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

func TestRunPluginMigrate_MissingEntryFailed(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	m := &managerImpl{}
	desc := Descriptor{
		Manifest: plugin_mgr.Manifest{
			ID:      "com.powerx.plugin.demo",
			Version: "0.0.1",
			Migrations: &plugin_mgr.MigrationsSpec{
				Entry: "backend/bin/migrate",
			},
		},
		Paths: plugin_mgr.InstalledPaths{
			Root:            root,
			MigrationsEntry: filepath.Join(root, "backend/bin/migrate"),
		},
	}

	rec, err := m.runPluginMigrate(context.Background(), desc, plugin_mgr.InstallOptions{RunMigrations: true})
	if err == nil {
		t.Fatal("runPluginMigrate() expected error when migration entry is missing")
	}
	if rec == nil {
		t.Fatal("runPluginMigrate() returned nil record")
	}
	if rec.LastStatus != plugin_mgr.MigrationStatusFailed {
		t.Fatalf("runPluginMigrate() status = %s, want %s", rec.LastStatus, plugin_mgr.MigrationStatusFailed)
	}
	if rec.LastError == "" {
		t.Fatal("runPluginMigrate() expected skip reason in LastError")
	}
}
