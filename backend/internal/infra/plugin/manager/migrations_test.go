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

func TestRunPluginMigrateDoesNotInheritCoreRuntimeEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}

	t.Setenv("POWERX_CONFIG", "/etc/powerx-dev/config.yaml")
	t.Setenv("POWERX_RUNTIME_ROOT", "/etc/powerx-dev")
	t.Setenv("POWERX_SETUP_RUNTIME_CONFIG_PATH", "/etc/powerx-dev/setup.wizard.config.json")

	root := t.TempDir()
	requireNoErr(t, os.MkdirAll(filepath.Join(root, "backend", "bin"), 0o755))

	outFile := filepath.Join(root, "env.out")
	script := filepath.Join(root, "backend", "bin", "migrate")
	requireNoErr(t, os.WriteFile(script, []byte("#!/bin/sh\nprintenv > \"$1\"\n"), 0o755))

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
		HostConfig: &plugin_mgr.HostConfig{
			Values: map[string]string{
				"PLUGIN_IAM_TENANT_KEY": "8a21845e-d1b6-4df1-b2ce-1d3bde3b8a03",
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
	got := string(raw)
	for _, denied := range []string{
		"POWERX_CONFIG=",
		"POWERX_RUNTIME_ROOT=",
		"POWERX_SETUP_RUNTIME_CONFIG_PATH=",
	} {
		if strings.Contains(got, denied) {
			t.Fatalf("plugin migration inherited denied env %s in:\n%s", denied, got)
		}
	}
	if !strings.Contains(got, "POWERX_PLUGIN_ID=com.powerx.plugin.demo") {
		t.Fatalf("plugin migration missing POWERX_PLUGIN_ID in:\n%s", got)
	}
	if !strings.Contains(got, "PLUGIN_IAM_TENANT_KEY=8a21845e-d1b6-4df1-b2ce-1d3bde3b8a03") {
		t.Fatalf("plugin migration missing tenant env in:\n%s", got)
	}
}

func TestRunPluginMigrateRejectsDelegatedPublicSchema(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}

	root := t.TempDir()
	requireNoErr(t, os.MkdirAll(filepath.Join(root, "backend", "bin"), 0o755))

	outFile := filepath.Join(root, "executed.out")
	script := filepath.Join(root, "backend", "bin", "migrate")
	requireNoErr(t, os.WriteFile(script, []byte("#!/bin/sh\nprintf executed > \"$1\"\n"), 0o755))

	m := &managerImpl{}
	desc := Descriptor{
		Manifest: plugin_mgr.Manifest{
			ID:      "com.powerx.plugin.demo",
			Version: "0.0.1",
			Runtime: plugin_mgr.RuntimeSpec{
				Env: map[string]string{
					"POWERX_PROVIDER_MODE":    "delegated",
					"POWERX_PROXY":            "1",
					"POWERX_PLUGIN_DB_SCHEMA": "public",
					"POWERX_DB_SCHEMA":        "public",
				},
			},
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
	if err == nil {
		t.Fatal("runPluginMigrate() expected error for delegated public schema")
	}
	if rec == nil || rec.LastStatus != plugin_mgr.MigrationStatusFailed {
		t.Fatalf("runPluginMigrate() record = %+v, want failed", rec)
	}
	if !strings.Contains(rec.LastError, "unsafe schema") {
		t.Fatalf("runPluginMigrate() LastError = %q, want unsafe schema", rec.LastError)
	}
	if _, statErr := os.Stat(outFile); !os.IsNotExist(statErr) {
		t.Fatalf("migration script executed despite unsafe schema, stat err=%v", statErr)
	}
}

func TestRunPluginMigrateAllowsDelegatedPluginSchema(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script test")
	}

	root := t.TempDir()
	requireNoErr(t, os.MkdirAll(filepath.Join(root, "backend", "bin"), 0o755))

	outFile := filepath.Join(root, "env.out")
	script := filepath.Join(root, "backend", "bin", "migrate")
	requireNoErr(t, os.WriteFile(script, []byte("#!/bin/sh\nprintenv > \"$1\"\n"), 0o755))

	m := &managerImpl{}
	desc := Descriptor{
		Manifest: plugin_mgr.Manifest{
			ID:      "com.powerx.plugin.demo",
			Version: "0.0.1",
			Runtime: plugin_mgr.RuntimeSpec{
				Env: map[string]string{
					"POWERX_PROVIDER_MODE":    "delegated",
					"POWERX_PROXY":            "1",
					"POWERX_PLUGIN_DB_SCHEMA": "px_com_powerx_plugin_demo",
					"POWERX_DB_SCHEMA":        "px_com_powerx_plugin_demo",
				},
			},
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
	got := string(raw)
	for _, want := range []string{
		"POWERX_PLUGIN_DB_SCHEMA=px_com_powerx_plugin_demo",
		"POWERX_DB_SCHEMA=px_com_powerx_plugin_demo",
		"POWERX_PROVIDER_MODE=delegated",
		"POWERX_PROXY=1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("plugin migration missing %s in:\n%s", want, got)
		}
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
