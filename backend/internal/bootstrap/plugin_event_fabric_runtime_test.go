package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const installEventTenant = "00000000-0000-0000-0000-000000000001"

func installEventPlugin() pm.Plugin {
	return pm.Plugin{ID: "com.powerx.plugins.fixture", Version: "1.0.0", RequiredCapabilities: []string{"com.corex.eventfabric.service_publish"}}
}

func TestPluginEventPlanValidatesBeforeSeeding(t *testing.T) {
	for _, test := range []struct {
		name, principal, action string
		existing                map[string]string
		wantErr                 string
	}{
		{name: "declared_publish", principal: "{{plugin_id}}", action: "publish"},
		{name: "matching_manual", principal: "{{plugin_id}}", action: "publish", existing: map[string]string{"fixture.event.v1": installEventTenant + ".fixture.event.v1"}},
		{name: "conflicting_manual", principal: "{{plugin_id}}", action: "publish", existing: map[string]string{"fixture.event.v1": installEventTenant + ".manual.other.v1"}, wantErr: "BINDING_CONFLICT"},
		{name: "extra_manual", principal: "{{plugin_id}}", action: "publish", existing: map[string]string{"other.key": installEventTenant + ".manual.other.v1"}, wantErr: "BINDING_CONFLICT"},
		{name: "foreign_principal", principal: "com.powerx.plugins.foreign", action: "publish", wantErr: "ACL_OWNER_INVALID"},
		{name: "undeclared_consume", principal: "{{plugin_id}}", action: "subscribe", wantErr: "ACL_ACTION_INVALID"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "service.yaml")
			require.NoError(t, os.WriteFile(path, []byte("version: v1\ntopics:\n- key: fixture.event.v1\n  namespace: fixture.event\n  name: v1\n  acl:\n  - principal_type: plugin\n    principal_id: '"+test.principal+"'\n    actions: ["+test.action+"]\n"), 0600))
			plan, seed, actions, err := pluginEventSeedPlan(installEventPlugin(), installEventTenant, path, test.existing)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				require.Nil(t, plan)
				return
			}
			require.NoError(t, err)
			require.Equal(t, installEventTenant, seed.TenantUUID)
			require.Equal(t, installEventTenant+".fixture.event.v1", plan.Topics[0].FullTopic)
			require.Equal(t, []string{"publish"}, actions["fixture.event.v1"])
		})
	}
}

type installEventReadProbe struct {
	record               *model.TopicDefinition
	allowed              bool
	err                  error
	queries, authQueries int
}

func (p *installEventReadProbe) FindTopicByFullName(_ context.Context, tenant, namespace, name string) (*model.TopicDefinition, error) {
	p.queries++
	if tenant != installEventTenant || namespace != "fixture.event" || name != "v1" {
		return nil, errors.New("FIXTURE_LOOKUP_INVALID")
	}
	return p.record, p.err
}
func (p *installEventReadProbe) HasPermission(_ context.Context, tenant string, id uuid.UUID, principal, action string, _ time.Time) (bool, error) {
	p.authQueries++
	if tenant != installEventTenant || principal != "com.powerx.plugins.fixture" || action != "publish" || id != p.record.UUID {
		return false, errors.New("FIXTURE_ACL_INVALID")
	}
	return p.allowed, p.err
}
func TestPluginEventBindingsRequireLiveDirectoryAndACL(t *testing.T) {
	full := installEventTenant + ".fixture.event.v1"
	for _, name := range []string{"valid", "missing", "retired", "revoked", "directory_error", "foreign_tenant", "duplicate"} {
		t.Run(name, func(t *testing.T) {
			probe := &installEventReadProbe{record: &model.TopicDefinition{PowerUUIDModel: coremodel.PowerUUIDModel{UUID: uuid.New()}, TenantKey: installEventTenant, Namespace: "fixture.event", Name: "v1", FullTopic: full, Lifecycle: model.TopicLifecycleActive}, allowed: true}
			bindings := map[string]string{"fixture.event.v1": full}
			switch name {
			case "missing":
				probe.record = nil
			case "retired":
				probe.record.Lifecycle = model.TopicLifecycleRetired
			case "revoked":
				probe.allowed = false
			case "directory_error":
				probe.err = errors.New("FIXTURE_DIRECTORY_UNAVAILABLE")
			case "foreign_tenant":
				bindings["fixture.event.v1"] = "00000000-0000-0000-0000-000000000002.fixture.event.v1"
			case "duplicate":
				bindings["other.key"] = full
			}
			got, err := verifyInstalledEventBindings(context.Background(), probe, probe, installEventPlugin().ID, installEventTenant, bindings, map[string][]string{"fixture.event.v1": {"publish"}, "other.key": {"publish"}})
			if name != "valid" {
				require.Error(t, err)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, bindings, got)
			require.Equal(t, 1, probe.queries)
			require.Equal(t, 1, probe.authQueries)
		})
	}
}
func TestPluginEventBindingsPreserveExplicitConfiguration(t *testing.T) {
	p := installEventPlugin()
	p.HostConfig = &pm.HostConfig{Spec: map[string]any{"runtime": map[string]any{"event_bridge": map[string]any{"topic_bindings": map[string]any{"fixture.event.v1": installEventTenant + ".fixture.event.v1"}}}}}
	got, err := explicitEventBindings(p)
	require.NoError(t, err)
	require.Len(t, got, 1)
	p.HostConfig.Spec["event_bridge"] = map[string]any{"topic_bindings": map[string]any{"fixture.event.v1": installEventTenant + ".manual.other"}}
	_, err = explicitEventBindings(p)
	require.ErrorContains(t, err, "BINDING_CONFLICT")
}
