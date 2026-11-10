package pluginpermission

import (
	"errors"
	"strings"
)

// APIKeySpec is an explicit plugin service grant. It is never inferred from an
// admin permission, a protocol auth_type, or the caller's display name.
type APIKeySpec struct {
	CapabilityID    string `yaml:"capability_id" json:"capability_id"`
	Scope           string `yaml:"scope" json:"scope"`
	Action          string `yaml:"action" json:"action"`
	ResourceType    string `yaml:"resource_type" json:"resource_type"`
	ResourcePattern string `yaml:"resource_pattern" json:"resource_pattern"`
	PluginID        string `yaml:"plugin_id" json:"plugin_id"`
	Effect          string `yaml:"effect" json:"effect"`
}

func ExplicitAPIKeyMetadata(spec *APIKeySpec, pluginID, capabilityID string, serviceBinding bool) (map[string]any, error) {
	if spec == nil {
		return nil, nil
	}
	invalid := errors.New("PLUGIN_API_KEY_DECLARATION_INVALID")
	if !serviceBinding || spec.PluginID != pluginID || !strings.HasPrefix(spec.CapabilityID, pluginID+".") || spec.Effect != "allow" {
		return nil, invalid
	}
	if capabilityID != "" && spec.CapabilityID != capabilityID {
		return nil, invalid
	}
	if !strings.HasPrefix(spec.Scope, "_scope.") || (spec.ResourceType != "api" && spec.ResourceType != "capability") {
		return nil, invalid
	}
	for _, value := range []string{spec.CapabilityID, spec.Scope, spec.Action, spec.ResourceType, spec.ResourcePattern, spec.PluginID} {
		if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "*\x00\r\n\t ") {
			return nil, invalid
		}
	}
	return map[string]any{
		"capability_id": spec.CapabilityID, "api_key_explicit": true,
		"api_key": map[string]any{"scope": spec.Scope, "action": spec.Action, "resource_type": spec.ResourceType, "resource_pattern": spec.ResourcePattern, "plugin_id": spec.PluginID, "effect": spec.Effect},
	}, nil
}
