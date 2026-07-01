package manager

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	pmrouter "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager/router"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

// MountDebugHost registers an externally started plugin runtime into the /_p router.
// It is intentionally keyed by the exact pluginID supplied by the caller: a local
// runtime such as com.powerx.plugins.base.local is a different plugin from the
// installed com.powerx.plugins.base runtime.
func MountDebugHost(mgr plugin_mgr.Manager, pluginID string, httpPort int) error {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return errors.New("plugin_id is required")
	}
	if httpPort <= 0 {
		return errors.New("http_port is required")
	}
	impl, ok := mgr.(*managerImpl)
	if !ok || impl == nil || impl.http == nil {
		return errors.New("plugin manager dynamic router is required")
	}
	target, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", httpPort))
	if err != nil {
		return err
	}
	impl.http.MountAPIProxy(pluginID, target, "/api/v1", "/healthz")
	impl.http.MountAdminProxy(pluginID, target)
	impl.http.InstallPolicy(pluginID, debugHostPolicy())
	return nil
}

func debugHostPolicy() *pmrouter.Policy {
	return &pmrouter.Policy{
		HTTPBase: "/api/v1",
		Routes: map[string]pmrouter.Permission{
			"GET:/api/v1/healthz":                          {Resource: "system", Action: "read"},
			"HEAD:/api/v1/healthz":                         {Resource: "system", Action: "read"},
			"GET:/api/v1/plugin/agent-registry/*":          {Resource: "agent_registry", Action: "read"},
			"POST:/api/v1/plugin/agent-registry/*":         {Resource: "agent_registry", Action: "create"},
			"POST:/api/v1/integration/capabilities/invoke": {Resource: "capability", Action: "create"},
			"GET:/api/v1/plugin/agent/sessions":            {Resource: "agent", Action: "read"},
			"POST:/api/v1/plugin/agent/sessions":           {Resource: "agent", Action: "create"},
			"GET:/api/v1/plugin/agent/sessions/*":          {Resource: "agent", Action: "read"},
			"POST:/api/v1/plugin/agent/stream/sse":         {Resource: "agent", Action: "create"},
			"GET:/api/v1/plugin/agent/stream/sse":          {Resource: "agent", Action: "read"},
			"GET:/api/v1/plugin/skills":                    {Resource: "skill", Action: "read"},
			"POST:/api/v1/plugin/skills/*":                 {Resource: "skill", Action: "create"},
		},
		Resources: map[string]map[string]bool{
			"agent_registry": {"read": true, "create": true},
			"capability":     {"create": true},
			"agent":          {"read": true, "create": true},
			"skill":          {"read": true, "create": true},
			"system":         {"read": true},
		},
	}
}

func UnmountDebugHost(mgr plugin_mgr.Manager, pluginID string) error {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return errors.New("plugin_id is required")
	}
	impl, ok := mgr.(*managerImpl)
	if !ok || impl == nil || impl.http == nil {
		return errors.New("plugin manager dynamic router is required")
	}
	impl.http.Unmount(pluginID)
	return nil
}
