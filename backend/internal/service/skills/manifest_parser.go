package skills

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type parsedSkillFrontmatter struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	Version     string      `yaml:"version"`
	Entrypoints interface{} `yaml:"entrypoints"`
}

func ParseSkillMarkdownToManifest(markdown string, fallbackVersion string) (map[string]interface{}, error) {
	frontmatter, err := extractYAMLFrontmatter(markdown)
	if err != nil {
		return nil, err
	}
	var parsed parsedSkillFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatter), &parsed); err != nil {
		return nil, fmt.Errorf("parse SKILL.md frontmatter failed: %w", err)
	}
	parsed.Name = strings.TrimSpace(parsed.Name)
	parsed.Description = strings.TrimSpace(parsed.Description)
	parsed.Version = strings.TrimSpace(parsed.Version)
	if parsed.Name == "" || parsed.Description == "" {
		return nil, errors.New("SKILL.md frontmatter requires name and description")
	}
	if parsed.Version == "" {
		parsed.Version = strings.TrimSpace(fallbackVersion)
	}
	entrypoints := normalizeEntrypoints(parsed.Entrypoints)
	if len(entrypoints) == 0 {
		entrypoints = []string{"runbook.default"}
	}
	return map[string]interface{}{
		"name":        parsed.Name,
		"description": parsed.Description,
		"version":     parsed.Version,
		"entrypoints": entrypoints,
		"schema":      "claude-code-skill",
	}, nil
}

func NormalizeManifestJSON(raw []byte, fallbackVersion string) (map[string]interface{}, error) {
	if len(raw) == 0 {
		return map[string]interface{}{
			"name":        "unknown-skill",
			"description": "no manifest provided",
			"version":     strings.TrimSpace(fallbackVersion),
			"entrypoints": []string{"runbook.default"},
			"schema":      "powerx-fallback",
		}, nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("invalid manifest json: %w", err)
	}
	name := strings.TrimSpace(toString(obj["name"]))
	desc := strings.TrimSpace(toString(obj["description"]))
	version := strings.TrimSpace(toString(obj["version"]))
	if version == "" {
		version = strings.TrimSpace(fallbackVersion)
	}
	entrypoints := normalizeEntrypoints(obj["entrypoints"])
	if len(entrypoints) == 0 {
		entrypoints = []string{"runbook.default"}
	}
	if name == "" {
		name = "unknown-skill"
	}
	if desc == "" {
		desc = "no description"
	}
	return map[string]interface{}{
		"name":        name,
		"description": desc,
		"version":     version,
		"entrypoints": entrypoints,
		"schema":      toString(obj["schema"]),
	}, nil
}

func extractYAMLFrontmatter(markdown string) (string, error) {
	content := strings.TrimSpace(markdown)
	if !strings.HasPrefix(content, "---") {
		return "", errors.New("SKILL.md must start with YAML frontmatter '---'")
	}
	lines := strings.Split(content, "\n")
	if len(lines) < 3 {
		return "", errors.New("SKILL.md frontmatter is incomplete")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end <= 1 {
		return "", errors.New("SKILL.md frontmatter closing '---' not found")
	}
	return strings.Join(lines[1:end], "\n"), nil
}

func normalizeEntrypoints(raw interface{}) []string {
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s := strings.TrimSpace(toString(item))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case map[string]interface{}:
		out := make([]string, 0, len(v))
		for key := range v {
			k := strings.TrimSpace(key)
			if k != "" {
				out = append(out, k)
			}
		}
		return out
	default:
		return nil
	}
}

func toString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		return fmt.Sprintf("%v", x)
	}
}

