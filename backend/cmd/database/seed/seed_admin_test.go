package seed

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSetupAdminFromDraftUsesRuntimeConfigDir(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(configPath, []byte("database: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	draftPath := filepath.Join(tmp, "setup.wizard.config.json")
	if err := os.WriteFile(draftPath, []byte(`{"admin":{"username":"root-admin","email":"root@example.test","password":"secret-123","display_name":"Root Admin","phone":"13900000000"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("POWERX_CONFIG", configPath)
	t.Setenv("POWERX_SETUP_RUNTIME_CONFIG_PATH", "")
	t.Setenv("POWERX_SETUP_DRAFT_PATH", "")
	t.Setenv("POWERX_RUNTIME_ROOT", "")

	admin, hasDraft, hasPassword := loadSetupAdminFromDraft()
	if !hasDraft {
		t.Fatal("expected setup draft")
	}
	if !hasPassword {
		t.Fatal("expected setup admin password")
	}
	if admin.Email != "root@example.test" || admin.Password != "secret-123" || admin.Username != "root-admin" {
		t.Fatalf("unexpected admin payload: %+v", admin)
	}
}

func TestLoadSetupAdminFromDraftUsesExplicitDraftPathFirst(t *testing.T) {
	tmp := t.TempDir()
	runtimeDir := filepath.Join(tmp, "runtime")
	explicitDir := filepath.Join(tmp, "explicit")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(explicitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(runtimeDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("database: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "setup.wizard.config.json"), []byte(`{"admin":{"email":"runtime@example.test","password":"runtime-pass"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	explicitDraft := filepath.Join(explicitDir, "setup.wizard.config.json")
	if err := os.WriteFile(explicitDraft, []byte(`{"admin":{"email":"explicit@example.test","password":"explicit-pass"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("POWERX_SETUP_DRAFT_PATH", explicitDraft)
	t.Setenv("POWERX_CONFIG", configPath)
	t.Setenv("POWERX_SETUP_RUNTIME_CONFIG_PATH", "")
	t.Setenv("POWERX_RUNTIME_ROOT", "")

	admin, hasDraft, hasPassword := loadSetupAdminFromDraft()
	if !hasDraft || !hasPassword {
		t.Fatalf("expected explicit setup draft with password, hasDraft=%t hasPassword=%t", hasDraft, hasPassword)
	}
	if admin.Email != "explicit@example.test" || admin.Password != "explicit-pass" {
		t.Fatalf("explicit draft was not preferred: %+v", admin)
	}
}

func TestLoadSetupAdminFromDraftWithoutPasswordSkipsDefaultRoot(t *testing.T) {
	tmp := t.TempDir()
	draftPath := filepath.Join(tmp, "setup.wizard.config.json")
	if err := os.WriteFile(draftPath, []byte(`{"admin":{"email":"root@example.test"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("POWERX_SETUP_DRAFT_PATH", draftPath)
	t.Setenv("POWERX_CONFIG", "")
	t.Setenv("POWERX_SETUP_RUNTIME_CONFIG_PATH", "")
	t.Setenv("POWERX_RUNTIME_ROOT", "")

	admin, hasDraft, hasPassword := loadSetupAdminFromDraft()
	if !hasDraft {
		t.Fatal("expected setup draft")
	}
	if hasPassword {
		t.Fatal("expected draft without admin password")
	}
	if admin.Email != "root@example.test" {
		t.Fatalf("unexpected admin payload: %+v", admin)
	}
}
