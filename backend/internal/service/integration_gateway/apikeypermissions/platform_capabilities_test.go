package apikeypermissions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	modelsiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
)

func TestBuildPlatformCapabilityPermissionsIncludesReadableI18nMeta(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`version: 1
capabilities:
  - capability_id: com.corex.example.records
    module: example
    title: Example Records
    description: Manage example records.
    permission_code: corex.example.records:manage
    title_i18n:
      zh-CN: 示例记录
      en: Example Records
    description_i18n:
      zh-CN: 管理示例记录。
      en: Manage example records.
    protocols:
      - channel: rest
        endpoint: /api/v1/admin/example/records
        method: POST
`)
	if err := os.WriteFile(filepath.Join(dir, "example.yaml"), raw, 0o644); err != nil {
		t.Fatalf("write test capability yaml: %v", err)
	}
	t.Setenv(platformCapabilitiesDirEnv, dir)
	platformPermissionOnce = sync.Once{}
	platformPermissionRows = nil
	platformPermissionErr = nil

	rows, err := BuildPlatformCapabilityPermissions()
	if err != nil {
		t.Fatalf("BuildPlatformCapabilityPermissions() error = %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	var operationRow, apiRow *modelsiam.Permission
	for i := range rows {
		var meta map[string]any
		if err := json.Unmarshal(rows[i].Meta, &meta); err != nil {
			t.Fatalf("unmarshal meta: %v", err)
		}
		switch meta["type"] {
		case "action":
			operationRow = &rows[i]
		case "api":
			apiRow = &rows[i]
		}
	}
	if operationRow == nil {
		t.Fatal("operation permission row missing")
	}
	if apiRow == nil {
		t.Fatal("api permission row missing")
	}
	if operationRow.Resource != "records" || operationRow.Action != "manage" {
		t.Fatalf("operation row = %s:%s", operationRow.Resource, operationRow.Action)
	}
	row := apiRow
	if row.Source != platformPermissionSource {
		t.Fatalf("Source = %q, want %q", row.Source, platformPermissionSource)
	}
	if row.Status != "active" {
		t.Fatalf("Status = %q, want active", row.Status)
	}
	if row.Description != "管理示例记录。" {
		t.Fatalf("Description = %q", row.Description)
	}

	var meta map[string]any
	if err := json.Unmarshal(row.Meta, &meta); err != nil {
		t.Fatalf("unmarshal meta: %v", err)
	}
	if meta["label"] != "示例记录" {
		t.Fatalf("meta.label = %#v", meta["label"])
	}
	if meta["permission_code"] != "corex.example.records:manage" {
		t.Fatalf("meta.permission_code = %#v", meta["permission_code"])
	}
	titleI18n, ok := meta["title_i18n"].(map[string]any)
	if !ok || titleI18n["zh-CN"] != "示例记录" || titleI18n["en"] != "Example Records" {
		t.Fatalf("meta.title_i18n = %#v", meta["title_i18n"])
	}
}

func TestBuildPlatformCapabilityPermissionsRejectsFormalAPIWithoutI18n(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`version: 1
capabilities:
  - capability_id: com.corex.example.records
    module: example
    title: Example Records
    description: Manage example records.
    permission_code: corex.example.records:manage
    protocols:
      - channel: rest
        endpoint: /api/v1/admin/example/records
        method: POST
`)
	if err := os.WriteFile(filepath.Join(dir, "example.yaml"), raw, 0o644); err != nil {
		t.Fatalf("write test capability yaml: %v", err)
	}
	t.Setenv(platformCapabilitiesDirEnv, dir)
	platformPermissionOnce = sync.Once{}
	platformPermissionRows = nil
	platformPermissionErr = nil

	_, err := BuildPlatformCapabilityPermissions()
	if err == nil {
		t.Fatal("BuildPlatformCapabilityPermissions() error = nil, want i18n validation error")
	}
	if got := err.Error(); !strings.Contains(got, "title_i18n is required") {
		t.Fatalf("error = %q, want title_i18n validation", got)
	}
}

func TestBuildPlatformCapabilityPermissionsDowngradesGeneratedAutoCapabilities(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`version: 1
capabilities:
  - capability_id: com.corex.rest.example.gin.get_api_v1_admin_example_records
    module: example
    title: GET /api/v1/admin/example/records
    description: Generated from Gin route source
    permission_code: corex.rest.example.gin:get_api_v1_admin_example_records
    protocols:
      - channel: rest
        endpoint: /api/v1/admin/example/records
        method: GET
`)
	if err := os.WriteFile(filepath.Join(dir, "generated.auto.yaml"), raw, 0o644); err != nil {
		t.Fatalf("write generated capability yaml: %v", err)
	}
	t.Setenv(platformCapabilitiesDirEnv, dir)
	platformPermissionOnce = sync.Once{}
	platformPermissionRows = nil
	platformPermissionErr = nil

	rows, err := BuildPlatformCapabilityPermissions()
	if err != nil {
		t.Fatalf("BuildPlatformCapabilityPermissions() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.Status != "deprecated" {
		t.Fatalf("Status = %q, want deprecated", row.Status)
	}
	if row.Source != "platform_capability_generated" {
		t.Fatalf("Source = %q, want platform_capability_generated", row.Source)
	}
	if row.AllowAPIKey {
		t.Fatal("AllowAPIKey = true, want false")
	}

	var meta map[string]any
	if err := json.Unmarshal(row.Meta, &meta); err != nil {
		t.Fatalf("unmarshal meta: %v", err)
	}
	if meta["type"] != "api_candidate" {
		t.Fatalf("meta.type = %#v, want api_candidate", meta["type"])
	}
	if _, ok := meta["label"]; ok {
		t.Fatalf("meta.label should not be set for generated candidates: %#v", meta["label"])
	}
}
