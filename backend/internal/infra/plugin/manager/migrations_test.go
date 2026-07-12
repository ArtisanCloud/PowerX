package manager

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestRunPluginMigrateSetsPluginSkillsDirWhenPackaged(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}

	root := t.TempDir()
	requireNoErr(t, os.MkdirAll(filepath.Join(root, "backend", "bin"), 0o755))
	requireNoErr(t, os.MkdirAll(filepath.Join(root, "skills", "template"), 0o755))
	requireNoErr(t, os.WriteFile(filepath.Join(root, "skills", "template", "SKILL.md"), []byte("---\nname: template\n---\n"), 0o644))

	outFile := filepath.Join(root, "env.out")
	script := filepath.Join(root, "backend", "bin", "migrate")
	requireNoErr(t, os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$PLUGIN_SKILLS_DIR\" > \"$1\"\n"), 0o755))

	m := &managerImpl{}
	desc := Descriptor{
		Manifest: plugin_mgr.Manifest{
			ID:      "com.powerx.plugin.demo",
			Version: "0.0.1",
			Migrations: &plugin_mgr.MigrationsSpec{
				Entry: "backend/bin/migrate",
				Args:  []string{outFile},
			},
		},
		Paths: plugin_mgr.InstalledPaths{
			Root:            root,
			MigrationsEntry: script,
		},
	}

	rec, err := m.runPluginMigrate(context.Background(), desc, plugin_mgr.InstallOptions{RunMigrations: true})
	if err != nil {
		t.Fatalf("runPluginMigrate() error = %v", err)
	}
	if rec == nil || rec.LastStatus != plugin_mgr.MigrationStatusSuccess {
		t.Fatalf("runPluginMigrate() record = %+v", rec)
	}
	raw, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read env output: %v", err)
	}
	got := strings.TrimSpace(string(raw))
	want := filepath.Join(root, "skills")
	if got != want {
		t.Fatalf("PLUGIN_SKILLS_DIR = %q, want %q", got, want)
	}
}

func TestInjectPluginSkillsDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	requireNoErr(t, os.MkdirAll(filepath.Join(root, "skills", "template"), 0o755))
	requireNoErr(t, os.WriteFile(filepath.Join(root, "skills", "template", "SKILL.md"), []byte("---\nname: template\n---\n"), 0o644))

	env := map[string]string{}
	injectPluginSkillsDir(env, root)

	if got := env["PLUGIN_SKILLS_DIR"]; got != filepath.Join(root, "skills") {
		t.Fatalf("PLUGIN_SKILLS_DIR = %q, want %q", got, filepath.Join(root, "skills"))
	}
}

func requireNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
