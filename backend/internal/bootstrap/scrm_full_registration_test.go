package bootstrap

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	pmimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	cm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	im "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func stageSCRMFullRegistrationArtifact(t *testing.T, source string) string {
	t.Helper()
	root := t.TempDir()
	copyFile := func(relative string) {
		raw, err := os.ReadFile(filepath.Join(source, relative))
		require.NoError(t, err)
		target := filepath.Join(root, relative)
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0700))
		require.NoError(t, os.WriteFile(target, raw, 0600))
	}
	copyFile("plugin.yaml")
	for _, directory := range []string{"plugin.d", "contracts", "docs/contracts", "web-admin/i18n"} {
		require.NoError(t, filepath.WalkDir(filepath.Join(source, directory), func(path string, entry fs.DirEntry, err error) error {
			require.NoError(t, err)
			if entry.IsDir() {
				return nil
			}
			require.Zero(t, entry.Type()&os.ModeSymlink)
			relative, err := filepath.Rel(source, path)
			require.NoError(t, err)
			copyFile(relative)
			return nil
		}))
	}
	for _, relative := range []string{"backend/bin/plugin", "backend/bin/migrate"} {
		target := filepath.Join(root, relative)
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0700))
		require.NoError(t, os.WriteFile(target, []byte("registration test placeholder; never executed"), 0600))
	}
	for _, relative := range []string{"web-admin/.output/public", "web-admin/public"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, relative), 0700))
	}
	return root
}

func TestSCRMCompleteManifestRegistersAndResyncsAllCapabilities(t *testing.T) {
	const prefix = "com.powerx.plugins.scrm."
	const tenant = "11111111-1111-1111-1111-111111111111"
	sources := []string{"../service/capability_registry/testdata/scrm-full-registration"}
	actual := os.Getenv("POWERX_SCRM_FULL_REGISTRATION_ROOT")
	if actual == "" {
		actual = "../../../../Plugins/com.powerx.plugin.scrm"
	}
	if info, err := os.Stat(filepath.Join(actual, "plugin.yaml")); err == nil && !info.IsDir() {
		sources = append(sources, actual)
	}
	for _, source := range sources {
		t.Run(filepath.Base(source), func(t *testing.T) {
			root := stageSCRMFullRegistrationArtifact(t, source)
			descriptor, err := pmimpl.NewFSLoader().LoadDescriptor(context.Background(), root)
			require.NoError(t, err)
			var manifest struct {
				Capabilities struct {
					Provides []struct{ ID, Descriptor string }
				}
			}
			raw, err := os.ReadFile(filepath.Join(root, "plugin.d/capabilities.yaml"))
			require.NoError(t, err)
			require.NoError(t, yaml.Unmarshal(raw, &manifest))
			require.GreaterOrEqual(t, len(manifest.Capabilities.Provides), 27)
			expected := map[string]int{}
			for _, provide := range manifest.Capabilities.Provides {
				raw, err := os.ReadFile(filepath.Join(root, provide.Descriptor))
				require.NoError(t, err)
				var doc struct {
					Metadata struct{ Protocols map[string]any }
				}
				require.NoError(t, yaml.Unmarshal(raw, &doc))
				count := 0
				for _, value := range doc.Metadata.Protocols {
					if bindings, ok := value.([]any); ok {
						count += len(bindings)
					} else {
						count++
					}
				}
				expected[provide.ID] = count
			}
			for _, suffix := range []string{"leads.read", "leads.write", "runtime_ops.invoke", "social_channel_accounts.read", "social_channel_accounts.write"} {
				require.Greater(t, expected[prefix+suffix], 1, suffix)
			}
			db := newPluginAuthzTestDB(t)
			require.NoError(t, db.AutoMigrate(&im.Role{}, &im.RolePermission{}, &cm.CapabilityRecord{}, &cm.CapabilityRegistration{}, &cm.AdapterEndpoint{}, &cm.RoutingPolicy{}, &cm.FallbackPlan{}, &cm.WorkflowTemplateRef{}, &cm.CapabilitySyncJob{}))
			worker := cap.NewSyncWorker(cap.SyncWorkerConfig{DB: db, TenantUUID: tenant})
			previousIDs := map[string][]string{}
			previousHash := map[string]string{}
			previousProtocolHash := map[string]string{}
			for repeat := 0; repeat < 2; repeat++ {
				require.NoError(t, syncPluginManifestPermissions(context.Background(), db, descriptor.Manifest))
				require.NoError(t, worker.ProcessArtifact(context.Background(), root), "the entire catalog must succeed")
				for id, count := range expected {
					var record cm.CapabilityRecord
					require.NoError(t, db.Where("capability_id=?", id).First(&record).Error)
					if repeat == 0 {
						previousHash[id] = record.CapabilitiesHash
						previousProtocolHash[id] = record.ProtocolHash
					} else {
						require.Equal(t, previousHash[id], record.CapabilitiesHash, id)
						require.Equal(t, previousProtocolHash[id], record.ProtocolHash, id)
					}
					var registration cm.CapabilityRegistration
					require.NoError(t, db.Where("capability_id=? AND tenant_uuid=?", id, tenant).Order("version DESC").First(&registration).Error)
					var adapters []cm.AdapterEndpoint
					require.NoError(t, db.Where("registration_id=?", registration.ID).Find(&adapters).Error)
					require.Len(t, adapters, count, id)
					seen := map[string]bool{}
					ids := make([]string, 0, len(adapters))
					for _, adapter := range adapters {
						require.False(t, seen[adapter.AdapterID], id)
						seen[adapter.AdapterID] = true
						ids = append(ids, adapter.AdapterID)
					}
					sort.Strings(ids)
					if repeat == 0 {
						previousIDs[id] = ids
					} else {
						require.Equal(t, previousIDs[id], ids, id)
					}
					if strings.HasPrefix(id, prefix+"messaging.") {
						var protocols []cm.ProtocolBinding
						require.NoError(t, json.Unmarshal(record.Protocols, &protocols))
						require.Len(t, protocols, 1)
						require.Equal(t, cap.PluginServiceAuthType, protocols[0].AuthType)
						require.Equal(t, "service_actor", protocols[0].ActorContext)
						require.Equal(t, "tenant", protocols[0].ResourceScope)
						require.True(t, strings.HasPrefix(protocols[0].Endpoint, "/api/v1/service/"))
						require.Equal(t, "1.0.0", registration.ContractRef)
					}
				}
			}
			var recordCount, failedJobs int64
			require.NoError(t, db.Model(&cm.CapabilityRecord{}).Count(&recordCount).Error)
			require.EqualValues(t, len(expected), recordCount)
			require.NoError(t, db.Model(&cm.CapabilitySyncJob{}).Where("status=?", "failed").Count(&failedJobs).Error)
			require.Zero(t, failedJobs)
			// 安装 Loader 仍保留五项服务的独立显式权限及真实插件 owner。
			servicePermissions := 0
			for _, permission := range descriptor.Manifest.Permissions {
				if permission.APIKey == nil || !strings.HasPrefix(permission.APIKey.CapabilityID, prefix+"messaging.") {
					continue
				}
				servicePermissions++
				require.True(t, permission.Independent)
				require.Equal(t, "com.powerx.plugins.scrm", permission.APIKey.PluginID)
			}
			require.Equal(t, 5, servicePermissions)
			totalAdapters := 0
			for _, count := range expected {
				totalAdapters += count
			}
			t.Logf("complete catalog: %d capabilities, %d adapters, IDs/catalog/protocol hashes unchanged across two syncs", len(expected), totalAdapters)
		})
	}
}
