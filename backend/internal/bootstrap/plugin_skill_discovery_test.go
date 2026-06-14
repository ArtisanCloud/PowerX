package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

func TestPluginRequiresSkillDiscovery(t *testing.T) {
	root := t.TempDir()
	required, err := pluginRequiresSkillDiscovery(pm.Plugin{Paths: pm.InstalledPaths{Root: root}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if required {
		t.Fatalf("plugin without skills directory or agent_tools catalog should not require discovery")
	}

	withSkills := filepath.Join(root, "with-skills")
	if err := os.MkdirAll(filepath.Join(withSkills, "skills"), 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	required, err = pluginRequiresSkillDiscovery(pm.Plugin{Paths: pm.InstalledPaths{Root: withSkills}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !required {
		t.Fatalf("plugin with skills directory should require discovery")
	}

	required, err = pluginRequiresSkillDiscovery(pm.Plugin{Catalogs: pm.CatalogSpec{AgentTools: "./plugin.d/agent_tools.yaml"}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !required {
		t.Fatalf("plugin with agent_tools catalog should require discovery")
	}
}

func TestPluginRequiresSkillDiscoveryRejectsLegacyAgentsTools(t *testing.T) {
	_, err := pluginRequiresSkillDiscovery(pm.Plugin{
		ID: "com.powerx.plugins.legacy",
		Agents: []pm.AgentSpec{{
			ID: "legacy.agent",
		}},
	})
	if err == nil {
		t.Fatalf("legacy agents without skill bridge should fail fast")
	}

	_, err = pluginRequiresSkillDiscovery(pm.Plugin{
		ID: "com.powerx.plugins.legacy",
		Tools: []pm.ToolSpec{{
			ID: "legacy.tool",
		}},
	})
	if err == nil {
		t.Fatalf("legacy tools without skill bridge should fail fast")
	}
}
