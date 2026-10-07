package apikeypermissions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	modelsiam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
)

func TestBuildPlatformCapabilityPermissionsIncludesAIModelCatalogAPIKeyGrants(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`version: 1
capabilities:
  - capability_id: com.corex.ai.catalog.providers.read
    module: ai
    title: AI Provider Catalog Read
    description: List AI providers.
    permission_code: corex.ai.catalog.providers:read
    title_i18n: {en: AI Provider Catalog Read, zh-CN: AI 提供商目录查询}
    description_i18n: {en: List AI providers., zh-CN: 查询 AI 提供商目录。}
    protocols:
      - channel: rest
        endpoint: /api/v1/admin/agents/providers
        method: GET
        api_key: {scope: _scope.ai.catalog.providers.read, action: read, resource_type: api, resource_pattern: ai-catalog-providers}
  - capability_id: com.corex.ai.catalog.models.read
    module: ai
    title: AI Model Catalog Read
    description: List AI models.
    permission_code: corex.ai.catalog.models:read
    title_i18n: {en: AI Model Catalog Read, zh-CN: AI 模型目录查询}
    description_i18n: {en: List AI models., zh-CN: 查询 AI 模型目录。}
    protocols:
      - channel: rest
        endpoint: /api/v1/admin/agents/models
        method: GET
        api_key: {scope: _scope.ai.catalog.models.read, action: read, resource_type: api, resource_pattern: ai-catalog-models}
`)
	if err := os.WriteFile(filepath.Join(dir, "ai_catalog.yaml"), raw, 0o644); err != nil {
		t.Fatalf("write capability yaml: %v", err)
	}
	t.Setenv(platformCapabilitiesDirEnv, dir)
	platformPermissionOnce = sync.Once{}
	platformPermissionRows = nil
	platformPermissionErr = nil

	rows, err := BuildPlatformCapabilityPermissions()
	if err != nil {
		t.Fatalf("BuildPlatformCapabilityPermissions() error = %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("len(rows) = %d, want 4", len(rows))
	}
	want := map[string]string{
		"com.corex.ai.catalog.providers.read": "_scope.ai.catalog.providers.read",
		"com.corex.ai.catalog.models.read":    "_scope.ai.catalog.models.read",
	}
	for _, row := range rows {
		if row.Status != modelsiam.PermissionStatusActive || !row.AllowAPIKey {
			continue
		}
		var meta map[string]any
		if err := json.Unmarshal(row.Meta, &meta); err != nil {
			t.Fatalf("unmarshal permission meta: %v", err)
		}
		capabilityID, _ := meta["capability_id"].(string)
		wantScope, ok := want[capabilityID]
		if !ok || meta["type"] != "api" {
			continue
		}
		resolved, ok := ResolvePermission(row)
		if !ok || resolved.Scope != wantScope || resolved.Action != "read" {
			t.Fatalf("capability %s permission = %#v, ok=%t", capabilityID, resolved, ok)
		}
		delete(want, capabilityID)
	}
	if len(want) != 0 {
		t.Fatalf("missing API Key capability permissions: %#v", want)
	}
}

func TestCheckedInAIModelCatalogCapabilitiesAreAPIKeyGrantable(t *testing.T) {
	configDir, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "config", "platform_capabilities"))
	if err != nil {
		t.Fatalf("resolve platform capability directory: %v", err)
	}
	t.Setenv(platformCapabilitiesDirEnv, configDir)
	rows, err := loadPlatformCapabilityPermissions()
	if err != nil {
		t.Fatalf("loadPlatformCapabilityPermissions() error = %v", err)
	}
	want := map[string]string{
		"com.corex.ai.catalog.providers.read": "_scope.ai.catalog.providers.read",
		"com.corex.ai.catalog.models.read":    "_scope.ai.catalog.models.read",
	}
	for _, row := range rows {
		var meta map[string]any
		if err := json.Unmarshal(row.Meta, &meta); err != nil {
			t.Fatalf("unmarshal permission meta: %v", err)
		}
		capabilityID, _ := meta["capability_id"].(string)
		wantScope, ok := want[capabilityID]
		if !ok || meta["type"] != "api" {
			continue
		}
		if row.Status != modelsiam.PermissionStatusActive || !row.AllowAPIKey {
			t.Fatalf("capability %s is not an active API Key permission", capabilityID)
		}
		resolved, ok := ResolvePermission(row)
		if !ok || resolved.Scope != wantScope || resolved.Action != "read" {
			t.Fatalf("capability %s permission = %#v, ok=%t", capabilityID, resolved, ok)
		}
		delete(want, capabilityID)
	}
	if len(want) != 0 {
		t.Fatalf("missing checked-in API Key capability permissions: %#v", want)
	}
}
