package manager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"gopkg.in/yaml.v3"
)

func loadManifestWithCatalogs(root string) (plugin_mgr.Manifest, error) {
	manifestPath := filepath.Join(root, PluginManifestFile)
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return plugin_mgr.Manifest{}, plugin_mgr.Wrap(
			plugin_mgr.CodeMissingFile,
			err,
			plugin_mgr.WithOp("load_manifest"),
			plugin_mgr.WithPath(manifestPath),
		)
	}

	var m plugin_mgr.Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return plugin_mgr.Manifest{}, plugin_mgr.Wrap(
			plugin_mgr.CodeInvalidManifest,
			err,
			plugin_mgr.WithOp("load_manifest"),
			plugin_mgr.WithPath(manifestPath),
		)
	}

	if isCatalogSpecEmpty(m.Catalogs) {
		return m, nil
	}

	var merged map[string]interface{}
	if err := yaml.Unmarshal(raw, &merged); err != nil {
		return plugin_mgr.Manifest{}, plugin_mgr.Wrap(
			plugin_mgr.CodeInvalidManifest,
			err,
			plugin_mgr.WithOp("merge_manifest_catalogs"),
			plugin_mgr.WithPath(manifestPath),
		)
	}
	if merged == nil {
		merged = map[string]interface{}{}
	}

	if err := applyCatalogSection(root, merged, "capabilities", m.Catalogs.Capabilities, []string{"capabilities"}); err != nil {
		return plugin_mgr.Manifest{}, err
	}
	if err := applyCatalogSection(root, merged, "exposure", m.Catalogs.Exposure, []string{"exposure"}); err != nil {
		return plugin_mgr.Manifest{}, err
	}
	if err := applyCatalogSection(root, merged, "agent_tools", m.Catalogs.AgentTools, []string{"agent_tools"}); err != nil {
		return plugin_mgr.Manifest{}, err
	}
	if err := applyCatalogSection(root, merged, "events", m.Catalogs.Events, []string{"events"}); err != nil {
		return plugin_mgr.Manifest{}, err
	}
	if err := applyCatalogSection(root, merged, "rbac", m.Catalogs.RBAC, []string{"rbac", "permissions", "routes"}); err != nil {
		return plugin_mgr.Manifest{}, err
	}

	mergedRaw, err := yaml.Marshal(merged)
	if err != nil {
		return plugin_mgr.Manifest{}, plugin_mgr.Wrap(
			plugin_mgr.CodeInvalidManifest,
			err,
			plugin_mgr.WithOp("merge_manifest_catalogs"),
			plugin_mgr.WithPath(manifestPath),
		)
	}

	var out plugin_mgr.Manifest
	if err := yaml.Unmarshal(mergedRaw, &out); err != nil {
		return plugin_mgr.Manifest{}, plugin_mgr.Wrap(
			plugin_mgr.CodeInvalidManifest,
			err,
			plugin_mgr.WithOp("merge_manifest_catalogs"),
			plugin_mgr.WithPath(manifestPath),
		)
	}
	return out, nil
}

func applyCatalogSection(root string, manifest map[string]interface{}, catalogName, relPath string, keys []string) error {
	relPath = strings.TrimSpace(relPath)
	if relPath == "" {
		return nil
	}
	if filepath.IsAbs(relPath) {
		return plugin_mgr.NewError(
			plugin_mgr.CodeInvalidManifest,
			plugin_mgr.WithOp("merge_manifest_catalogs"),
			plugin_mgr.WithMsg("catalog %s must use relative path: %s", catalogName, relPath),
		)
	}

	catalogPath := filepath.Join(root, filepath.Clean(relPath))
	if !strings.HasPrefix(filepath.Clean(catalogPath), filepath.Clean(root)) {
		return plugin_mgr.NewError(
			plugin_mgr.CodeInvalidManifest,
			plugin_mgr.WithOp("merge_manifest_catalogs"),
			plugin_mgr.WithMsg("catalog %s path escapes plugin root: %s", catalogName, relPath),
		)
	}
	raw, err := os.ReadFile(catalogPath)
	if err != nil {
		return plugin_mgr.Wrap(
			plugin_mgr.CodeMissingFile,
			err,
			plugin_mgr.WithOp("merge_manifest_catalogs"),
			plugin_mgr.WithPath(catalogPath),
		)
	}

	var catalog map[string]interface{}
	if err := yaml.Unmarshal(raw, &catalog); err != nil {
		return plugin_mgr.Wrap(
			plugin_mgr.CodeInvalidManifest,
			err,
			plugin_mgr.WithOp("merge_manifest_catalogs"),
			plugin_mgr.WithPath(catalogPath),
		)
	}
	if catalog == nil {
		catalog = map[string]interface{}{}
	}

	for _, key := range keys {
		val, found := findCatalogValue(catalog, key)
		if !found || isEmptyValue(val) {
			if key == "rbac" {
				if _, ok := catalog["resources"]; ok {
					val = catalog
					found = true
				}
			}
		}
		if !found || isEmptyValue(val) {
			continue
		}

		if existing, ok := manifest[key]; ok && !isEmptyValue(existing) {
			return plugin_mgr.NewError(
				plugin_mgr.CodeInvalidManifest,
				plugin_mgr.WithOp("merge_manifest_catalogs"),
				plugin_mgr.WithMsg("catalog conflict on field %q (catalog=%s)", key, catalogName),
			)
		}
		manifest[key] = val
	}

	for _, key := range keys {
		if _, ok := manifest[key]; ok {
			continue
		}
		return plugin_mgr.NewError(
			plugin_mgr.CodeInvalidManifest,
			plugin_mgr.WithOp("merge_manifest_catalogs"),
			plugin_mgr.WithMsg("catalog %s missing required field %q", catalogName, key),
		)
	}
	return nil
}

func findCatalogValue(catalog map[string]interface{}, key string) (interface{}, bool) {
	if v, ok := catalog[key]; ok {
		return v, true
	}
	if root, ok := catalog["manifest"].(map[string]interface{}); ok {
		if v, ok := root[key]; ok {
			return v, true
		}
	}
	return nil, false
}

func isCatalogSpecEmpty(c plugin_mgr.CatalogSpec) bool {
	return strings.TrimSpace(c.Capabilities) == "" &&
		strings.TrimSpace(c.Exposure) == "" &&
		strings.TrimSpace(c.AgentTools) == "" &&
		strings.TrimSpace(c.Events) == "" &&
		strings.TrimSpace(c.RBAC) == ""
}

func isEmptyValue(v interface{}) bool {
	if v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case []interface{}:
		return len(t) == 0
	case map[string]interface{}:
		return len(t) == 0
	default:
		return fmt.Sprintf("%v", v) == ""
	}
}

func persistMergedManifest(root string, m plugin_mgr.Manifest) error {
	yamlRaw, err := yaml.Marshal(m)
	if err != nil {
		return plugin_mgr.Wrap(
			plugin_mgr.CodeInvalidManifest,
			err,
			plugin_mgr.WithOp("persist_merged_manifest"),
		)
	}
	yamlPath := filepath.Join(root, "plugin.merged.yaml")
	if err := os.WriteFile(yamlPath, yamlRaw, 0o644); err != nil {
		return plugin_mgr.Wrap(
			plugin_mgr.CodeIOError,
			err,
			plugin_mgr.WithOp("persist_merged_manifest"),
			plugin_mgr.WithPath(yamlPath),
		)
	}

	jsonRaw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return plugin_mgr.Wrap(
			plugin_mgr.CodeInvalidManifest,
			err,
			plugin_mgr.WithOp("persist_merged_manifest"),
		)
	}
	jsonPath := filepath.Join(root, "plugin.merged.json")
	if err := os.WriteFile(jsonPath, jsonRaw, 0o644); err != nil {
		return plugin_mgr.Wrap(
			plugin_mgr.CodeIOError,
			err,
			plugin_mgr.WithOp("persist_merged_manifest"),
			plugin_mgr.WithPath(jsonPath),
		)
	}
	return nil
}
