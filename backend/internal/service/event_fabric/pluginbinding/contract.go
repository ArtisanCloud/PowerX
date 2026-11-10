package pluginbinding

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/autoseed"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/manifest"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"os"
	"strings"
	"time"
)

type Directory interface {
	FindTopicByFullName(context.Context, string, string, string) (*model.TopicDefinition, error)
}
type ACL interface {
	HasPermission(context.Context, string, uuid.UUID, string, string, time.Time) (bool, error)
}

func SeedPlan(plugin pm.Plugin, tenant, path string, existing map[string]string) (*manifest.SeedPlan, manifest.SeedContext, map[string][]string, error) {
	seedCtx := manifest.SeedContext{TenantUUID: tenant, PluginID: plugin.ID, PluginVersion: plugin.Version, Operator: "plugin-enable", Variables: autoseed.BuildSeedVariables(plugin)}
	file, err := os.Open(path)
	if err != nil {
		return nil, seedCtx, nil, err
	}
	defer file.Close()
	doc, err := manifest.Parse(file)
	if err != nil {
		return nil, seedCtx, nil, err
	}
	plan, err := doc.Render(seedCtx)
	if err != nil {
		return nil, seedCtx, nil, err
	}
	allowed := map[string]bool{}
	for _, capability := range plugin.RequiredCapabilities {
		if capability == "com.corex.eventfabric.service_publish" {
			allowed["publish"] = true
		}
		if capability == "com.corex.eventfabric.service_consume" {
			allowed["subscribe"] = true
		}
	}
	actions := make(map[string][]string, len(plan.Topics))
	wanted := map[string]string{}
	for _, topic := range plan.Topics {
		if topic.Key == "" || wanted[topic.Key] != "" {
			return nil, seedCtx, nil, fmt.Errorf("EVENT_FABRIC_INSTALL_KEY_INVALID")
		}
		wanted[topic.Key] = topic.FullTopic
		for _, grant := range topic.ACL {
			if grant.PrincipalType != "plugin" || grant.PrincipalID != plugin.ID {
				return nil, seedCtx, nil, fmt.Errorf("EVENT_FABRIC_INSTALL_ACL_OWNER_INVALID")
			}
			for _, action := range grant.Actions {
				if !allowed[string(action)] {
					return nil, seedCtx, nil, fmt.Errorf("EVENT_FABRIC_INSTALL_ACL_ACTION_INVALID")
				}
				actions[topic.Key] = append(actions[topic.Key], string(action))
			}
		}
		if len(actions[topic.Key]) == 0 {
			return nil, seedCtx, nil, fmt.Errorf("EVENT_FABRIC_INSTALL_ACL_REQUIRED: %s", topic.Key)
		}
	}
	if len(wanted) == 0 {
		return nil, seedCtx, nil, fmt.Errorf("EVENT_FABRIC_INSTALL_DECLARATION_REQUIRED")
	}
	// Reject conflicts before creating topics or granting any ACL.
	for key, full := range existing {
		if wanted[key] != full {
			return nil, seedCtx, nil, fmt.Errorf("EVENT_FABRIC_INSTALL_BINDING_CONFLICT: %s", key)
		}
	}
	return plan, seedCtx, actions, nil
}

func ExplicitBindings(plugin pm.Plugin) (map[string]string, error) {
	out := map[string]string{}
	if plugin.HostConfig == nil {
		return out, nil
	}
	spec := plugin.HostConfig.Spec
	if raw, exists := spec["event_bridge"]; exists {
		if _, ok := raw.(map[string]any); !ok {
			return nil, fmt.Errorf("EVENT_FABRIC_INSTALL_BINDINGS_INVALID")
		}
	}
	runtime, _ := spec["runtime"].(map[string]any)
	canonical, _ := runtime["event_bridge"].(map[string]any)
	root, _ := spec["event_bridge"].(map[string]any)
	for _, bridge := range []map[string]any{canonical, root} {
		raw, exists := bridge["topic_bindings"]
		if !exists {
			continue
		}
		bindings, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("EVENT_FABRIC_INSTALL_BINDINGS_INVALID")
		}
		for key, value := range bindings {
			full, ok := value.(string)
			if !ok || key == "" || key != strings.TrimSpace(key) || len(key) > 255 || strings.ContainsAny(key, "\r\n\x00") {
				return nil, fmt.Errorf("EVENT_FABRIC_INSTALL_BINDINGS_INVALID")
			}
			if prev, exists := out[key]; exists && prev != full {
				return nil, fmt.Errorf("EVENT_FABRIC_INSTALL_BINDING_CONFLICT: %s", key)
			}
			out[key] = full
		}
	}
	return out, nil
}

func Verify(ctx context.Context, dir Directory, acl ACL, pluginID, tenant string, bindings map[string]string, actions map[string][]string) (map[string]string, error) {
	if len(bindings) == 0 {
		return nil, fmt.Errorf("EVENT_FABRIC_INSTALL_DECLARATION_REQUIRED")
	}
	out := make(map[string]string, len(bindings))
	seen := map[string]bool{}
	for key, full := range bindings {
		relative, ok := strings.CutPrefix(full, tenant+".")
		split := strings.LastIndex(relative, ".")
		if !ok || split <= 0 || split == len(relative)-1 || full != strings.TrimSpace(full) || len(full) > 255 || strings.ContainsAny(full, "\r\n\x00") || seen[full] {
			return nil, fmt.Errorf("EVENT_FABRIC_INSTALL_BINDINGS_INVALID")
		}
		seen[full] = true
		record, err := dir.FindTopicByFullName(ctx, tenant, relative[:split], relative[split+1:])
		if err != nil {
			return nil, err
		}
		if record == nil || record.UUID == uuid.Nil || record.TenantKey != tenant || record.FullTopic != full || record.Lifecycle != model.TopicLifecycleActive {
			return nil, fmt.Errorf("EVENT_FABRIC_INSTALL_TOPIC_NOT_READY: %s", key)
		}
		if len(actions[key]) == 0 {
			return nil, fmt.Errorf("EVENT_FABRIC_INSTALL_ACL_REQUIRED: %s", key)
		}
		for _, action := range actions[key] {
			granted, err := acl.HasPermission(ctx, tenant, record.UUID, pluginID, action, time.Now().UTC())
			if err != nil {
				return nil, err
			}
			if !granted {
				return nil, fmt.Errorf("EVENT_FABRIC_INSTALL_ACL_REQUIRED: %s", key)
			}
		}
		out[key] = record.FullTopic
	}
	return out, nil
}
