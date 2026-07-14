package metadata_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

type metadataCapabilityFile struct {
	Capabilities []metadataCapability `yaml:"capabilities"`
}

type metadataCapability struct {
	CapabilityID   string                      `yaml:"capability_id"`
	PermissionCode string                      `yaml:"permission_code"`
	AgentUsable    *bool                       `yaml:"agent_usable"`
	RiskLevel      string                      `yaml:"risk_level"`
	Protocols      []metadataCapabilityBinding `yaml:"protocols"`
}

type metadataCapabilityBinding struct {
	Channel       string `yaml:"channel"`
	Endpoint      string `yaml:"endpoint"`
	Method        string `yaml:"method"`
	ActorContext  string `yaml:"actor_context"`
	ResourceScope string `yaml:"resource_scope"`
	STSDirect     bool   `yaml:"sts_direct"`
}

func TestMetadataCapabilitiesAreDeclaredForPluginConsumption(t *testing.T) {
	caps := loadMetadataCapabilities(t)
	required := map[string]string{
		"com.corex.metadata.dictionary.read":    "metadata.dictionary:read",
		"com.corex.metadata.taxonomy.read":      "metadata.taxonomy:read",
		"com.corex.metadata.tag.read":           "metadata.tag:read",
		"com.corex.metadata.tag.manage":         "metadata.tag:manage",
		"com.corex.metadata.resource_type.read": "metadata.resource_type:read",
	}
	for capabilityID, permissionCode := range required {
		capability, ok := caps[capabilityID]
		if !ok {
			t.Fatalf("missing metadata capability %s", capabilityID)
		}
		if capability.PermissionCode != permissionCode {
			t.Fatalf("capability %s permission_code=%s want %s", capabilityID, capability.PermissionCode, permissionCode)
		}
		if capability.AgentUsable == nil || *capability.AgentUsable {
			t.Fatalf("capability %s must explicitly set agent_usable=false", capabilityID)
		}
		if capability.RiskLevel == "" {
			t.Fatalf("capability %s missing risk_level", capabilityID)
		}
		if len(capability.Protocols) == 0 {
			t.Fatalf("capability %s missing protocol bindings", capabilityID)
		}
		for _, protocol := range capability.Protocols {
			if protocol.Channel == "rest" {
				if protocol.Endpoint == "" || protocol.Method == "" || protocol.ActorContext == "" || protocol.ResourceScope == "" {
					t.Fatalf("capability %s has incomplete REST protocol: %+v", capabilityID, protocol)
				}
				if protocol.ActorContext == "admin_user" && protocol.STSDirect {
					t.Fatalf("capability %s admin_user REST binding must not set sts_direct=true", capabilityID)
				}
			}
		}
	}
}

func loadMetadataCapabilities(t *testing.T) map[string]metadataCapability {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve test source path")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "config", "platform_capabilities", "metadata.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read metadata capabilities: %v", err)
	}
	var parsed metadataCapabilityFile
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse metadata capabilities: %v", err)
	}
	out := make(map[string]metadataCapability, len(parsed.Capabilities))
	for _, capability := range parsed.Capabilities {
		out[capability.CapabilityID] = capability
	}
	return out
}
