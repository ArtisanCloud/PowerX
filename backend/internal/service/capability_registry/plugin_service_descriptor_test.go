package capability_registry

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	iam "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gw "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const scrmFixtureRoot = "testdata/scrm-service-registration"
const scrmDescriptorDir = "specs/008-channel-provider-messaging/contracts/service-descriptors"
const scrmPluginID = "com.powerx.plugins.scrm"

func scrmDescriptors(t *testing.T, root string) []catalogCapability {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, scrmDescriptorDir, "*.yaml"))
	require.NoError(t, err)
	require.Len(t, files, 5)
	caps := make([]catalogCapability, 0, 5)
	for _, file := range files {
		relative, err := filepath.Rel(root, file)
		require.NoError(t, err)
		c, err := loadDescriptorCapability(root, capabilityManifestProvide{ID: strings.TrimSuffix(filepath.Base(file), ".yaml"), Version: "1.0.0", Descriptor: relative})
		require.NoError(t, err)
		caps = append(caps, c)
	}
	return caps
}
func writeSCRMServiceArtifact(t *testing.T, basePath string) string {
	t.Helper()
	root := t.TempDir()
	write := func(relative string, raw []byte) {
		dest := filepath.Join(root, relative)
		require.NoError(t, os.MkdirAll(filepath.Dir(dest), 0700))
		require.NoError(t, os.WriteFile(dest, raw, 0600))
	}
	files, err := filepath.Glob(filepath.Join(scrmFixtureRoot, scrmDescriptorDir, "*.yaml"))
	require.NoError(t, err)
	require.Len(t, files, 5)
	provides := []any{}
	permissions := []any{}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		require.NoError(t, err)
		var doc map[string]any
		require.NoError(t, yaml.Unmarshal(raw, &doc))
		if basePath != "/api/v1" {
			protocol := doc["metadata"].(map[string]any)["protocols"].(map[string]any)["rest"].(map[string]any)
			protocol["path"] = basePath + strings.TrimPrefix(protocol["path"].(string), "/api/v1")
			for _, p := range doc["permissions"].([]any) {
				for _, b := range p.(map[string]any)["protocol_bindings"].([]any) {
					binding := b.(map[string]any)
					binding["path"] = basePath + strings.TrimPrefix(binding["path"].(string), "/api/v1")
				}
			}
			raw, err = yaml.Marshal(doc)
			require.NoError(t, err)
		}
		relative := filepath.Join(scrmDescriptorDir, filepath.Base(file))
		write(relative, raw)
		provides = append(provides, map[string]any{"id": doc["id"], "version": "1.0.0", "descriptor": relative})
		permissions = append(permissions, doc["permissions"].([]any)...)
	}
	schema := "docs/contracts/messaging-service.openapi.yaml"
	raw, err := os.ReadFile(filepath.Join(scrmFixtureRoot, schema))
	require.NoError(t, err)
	write(schema, raw)
	raw, err = yaml.Marshal(map[string]any{"id": scrmPluginID, "name": "SCRM registration fixture", "version": "1.0.0", "runtime": map[string]any{"kind": "process", "entry": "bin/fixture"}, "endpoints": map[string]any{"http_base_path": basePath}, "permissions": permissions})
	require.NoError(t, err)
	write("plugin.yaml", raw)
	raw, err = yaml.Marshal(map[string]any{"capabilities": map[string]any{"provides": provides}})
	require.NoError(t, err)
	write("plugin.d/capabilities.yaml", raw)
	write("bin/fixture", []byte("test fixture; never executed"))
	return root
}
func TestSCRMFiveRealServiceDescriptorsValidate(t *testing.T) {
	sources := []string{scrmFixtureRoot}
	root := os.Getenv("POWERX_SCRM_SERVICE_DESCRIPTOR_ROOT")
	if root == "" {
		root = "../../../../../Plugins/com.powerx.plugin.scrm"
	}
	if info, err := os.Stat(filepath.Join(root, scrmDescriptorDir)); err == nil && info.IsDir() {
		sources = append(sources, root)
	}
	for _, root := range sources {
		t.Run(filepath.Base(root), func(t *testing.T) {
			for _, c := range scrmDescriptors(t, root) {
				require.NoError(t, validateServiceTicketProtocols(c.Protocols), c.ID)
				require.NoError(t, validatePluginPermissionDeclarations(scrmPluginID, c.ID, c.Protocols, c.Permissions), c.ID)
				require.NoError(t, validateCatalogAPIKeyPermissionOwnership(pluginMetadata{ID: scrmPluginID}, []catalogCapability{c}), c.ID)
			}
		})
	}
}
func TestSCRMServiceDescriptorsPassInstalledArtifactSync(t *testing.T) {
	for _, basePath := range []string{"/api/v1", "/custom/api/v7"} {
		t.Run(strings.ReplaceAll(basePath, "/", "_"), func(t *testing.T) {
			root := writeSCRMServiceArtifact(t, basePath)
			caps := scrmDescriptors(t, root)
			db := newMemoryDB(t)
			worker := NewSyncWorker(SyncWorkerConfig{DB: db, TenantUUID: grantStatusTestTenant})
			for repeat := 0; repeat < 2; repeat++ {
				require.NoError(t, worker.ProcessArtifact(context.Background(), root))
			}
			for _, c := range caps {
				record, err := worker.recordRepo.GetByCapabilityID(context.Background(), c.ID)
				require.NoError(t, err)
				var protocols []models.ProtocolBinding
				require.NoError(t, json.Unmarshal(record.Protocols, &protocols))
				require.Equal(t, c.Protocols, protocols)
				require.NotEmpty(t, record.CapabilitiesHash)
				require.NotEmpty(t, record.ProtocolHash)
				var registration models.CapabilityRegistration
				require.NoError(t, db.Where("capability_id=? AND tenant_uuid=?", c.ID, grantStatusTestTenant).Order("version DESC").First(&registration).Error)
				require.Equal(t, "1.0.0", registration.ContractRef)
				declaration := c.Permissions[0]
				var row iam.Permission
				require.NoError(t, db.Where("module=? AND resource=? AND action=?", declaration.Module, declaration.Resource, declaration.Action).First(&row).Error)
				require.True(t, row.AllowAPIKey)
				require.Equal(t, "plugin:"+scrmPluginID, row.Source)
				var meta map[string]any
				require.NoError(t, json.Unmarshal(row.Meta, &meta))
				require.Equal(t, true, meta["api_key_explicit"])
				require.Equal(t, true, meta["independent"])
				require.Equal(t, scrmPluginID, meta["api_key"].(map[string]any)["plugin_id"])
				require.Equal(t, c.Protocols[0].Endpoint, meta["protocol_bindings"].([]any)[0].(map[string]any)["path"])
			}
			var count int64
			require.NoError(t, db.Model(&iam.Permission{}).Where("allow_api_key = ?", true).Count(&count).Error)
			require.EqualValues(t, 5, count)
			// 正式权限目录登记不等于 Key 获得 grant；用无 grant 的服务 Key 验证拒绝。
			require.NoError(t, db.AutoMigrate(&iam.APIKeyProfile{}, &iam.APIKeyProfilePermission{}, &gw.IntegrationGatewayAPIKey{}, &gw.IntegrationGatewayAPIKeyPermission{}))
			profile := iam.APIKeyProfile{TenantUUID: grantStatusTestTenant, Key: "no-grant", Name: "fixture", Status: 1}
			require.NoError(t, db.Create(&profile).Error)
			key := gw.IntegrationGatewayAPIKey{TenantUUID: grantStatusTestTenant, ProfileID: profile.ID, Name: "ungranted", KeyHash: "synthetic-ungranted-key", Status: "active"}
			require.NoError(t, db.Create(&key).Error)
			require.NoError(t, db.Model(&models.CapabilityRecord{}).Where("plugin_id=?", scrmPluginID).Update("status", "published").Error)
			ctx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), grantStatusTestTenant), &reqctx.CoreXClaims{TenantUUID: grantStatusTestTenant, Platforms: []string{"api_key"}}), key.KeyHash)
			access, err := NewGrantStatusService(db).CurrentAccess(ctx)
			require.NoError(t, err)
			for _, c := range caps {
				require.Error(t, access.Require(c.ID))
			}
			require.NoError(t, db.Model(&gw.IntegrationGatewayAPIKeyPermission{}).Count(&count).Error)
			require.Zero(t, count)

		})
	}
}

func TestServiceTicketPermissionExceptionRejectsMismatchesAndUnsafePaths(t *testing.T) {
	tests := []struct {
		name  string
		alter func(*catalogCapability)
	}{
		{"permission path", func(c *catalogCapability) { c.Permissions[0].ProtocolBindings[0].Path += ".other" }},
		{"permission method", func(c *catalogCapability) { c.Permissions[0].ProtocolBindings[0].Method = "GET" }},
		{"missing schema", func(c *catalogCapability) { c.Protocols[0].SchemaRef = "" }},
		{"wrong channel", func(c *catalogCapability) { c.Protocols[0].Channel = "grpc" }},
		{"invalid method", func(c *catalogCapability) {
			c.Protocols[0].Method = "CONNECT"
			c.Permissions[0].ProtocolBindings[0].Method = "CONNECT"
		}},
		{"metadata admin", func(c *catalogCapability) { c.Protocols[0].ActorContext = "admin_user" }},
		{"permission admin", func(c *catalogCapability) { c.Permissions[0].ProtocolBindings[0].ActorContext = "admin_user" }},
		{"metadata global scope", func(c *catalogCapability) { c.Protocols[0].ResourceScope = "global" }},
		{"permission global scope", func(c *catalogCapability) { c.Permissions[0].ProtocolBindings[0].ResourceScope = "global" }},

		{"STS permission admin", func(c *catalogCapability) {
			c.Permissions[0].APIKey = nil
			c.Permissions[0].ProtocolBindings[0].ActorContext = "admin_user"
		}},
		{"STS permission global", func(c *catalogCapability) {
			c.Permissions[0].APIKey = nil
			c.Permissions[0].ProtocolBindings[0].ResourceScope = "global"
		}},
		{"STS permission not independent", func(c *catalogCapability) { c.Permissions[0].APIKey = nil; c.Permissions[0].Independent = false }},
		{"not independent", func(c *catalogCapability) { c.Permissions[0].Independent = false }},
		{"permission alias", func(c *catalogCapability) { c.Permissions[0].BusinessPermissionCode = "scrm.messages:reply" }},
		{"wrong plugin owner", func(c *catalogCapability) { c.Permissions[0].APIKey.PluginID = "com.example.other" }},
		{"capability owned elsewhere", func(c *catalogCapability) { c.Permissions[0].APIKey.CapabilityID = "com.example.other.reply" }},
		{"ordinary binding remains blocked", func(c *catalogCapability) { c.Protocols[0].AuthType = "tenant_jwt" }},
		{"ticket on another endpoint", func(c *catalogCapability) {
			ticket := c.Protocols[0]
			c.Protocols[0].AuthType = "tenant_jwt"
			ticket.Endpoint += ".ticket"
			c.Protocols = append(c.Protocols, ticket)
		}},
		{"ticket on another method", func(c *catalogCapability) {
			ticket := c.Protocols[0]
			c.Protocols[0].AuthType = "tenant_jwt"
			ticket.Method = "GET"
			c.Protocols = append(c.Protocols, ticket)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := scrmDescriptors(t, scrmFixtureRoot)[3]
			test.alter(&c)
			require.Error(t, validatePluginPermissionDeclarations(scrmPluginID, c.ID, c.Protocols, c.Permissions))
		})
	}
	for _, path := range []string{"https://example.invalid/service", "//example.invalid/service", "/_p/com.example/api/v1/service/reply", "/custom/_p/reply", "/api/v1/_p", "/api/v1/service?tenant=other", "/api/v1/service?", "/api/v1/service#", "/api/v1/{id}", "/api/v1/:id", "/api/v1/*", "/api/v1/../admin", "/api/v1/%2e%2e/admin", "/api/v1//service"} {
		t.Run(path, func(t *testing.T) {
			c := scrmDescriptors(t, scrmFixtureRoot)[3]
			c.Protocols[0].Endpoint = path
			c.Permissions[0].ProtocolBindings[0].Path = path
			require.Error(t, validatePluginPermissionDeclarations(scrmPluginID, c.ID, c.Protocols, c.Permissions))
		})
	}
}

func TestServiceTicketSchemaFragmentValidationRejectsBadReferences(t *testing.T) {
	root := scrmFixtureRoot
	capability := scrmDescriptors(t, root)[3]
	worker := NewSyncWorker(SyncWorkerConfig{DB: newMemoryDB(t)})
	for _, ref := range []string{"docs/contracts/missing.yaml#/components/schemas/ReplyRequest", "docs/contracts/messaging-service.openapi.yaml#/components/schemas/Unknown", "docs/contracts/messaging-service.openapi.yaml#/components/schemas/~2bad", "../outside.yaml#/schema", "/absolute.yaml#/schema", "docs/contracts/messaging-service.openapi.yaml?query=1#/schema"} {
		t.Run(ref, func(t *testing.T) {
			c := capability
			c.Protocols = append([]models.ProtocolBinding(nil), capability.Protocols...)
			c.Protocols[0].SchemaRef = ref
			require.Error(t, worker.validateSchemaRefs(context.Background(), root, root, pluginMetadata{ID: scrmPluginID}, c))
		})
	}
	require.NoError(t, validateServiceTicketSchemaDocument([]byte(`{"schemas":{"a/b~c":{"type":"object"}}}`), "/schemas/a~1b~0c"))
}
