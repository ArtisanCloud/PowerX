package skills

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type promptTemplateExecutor struct{}

var promptTemplateVarPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.-]+)\s*\}\}`)

func newPromptTemplateExecutor() SkillExecutor { return &promptTemplateExecutor{} }

func (e *promptTemplateExecutor) CanHandle(in ExecuteInput) bool {
	return strings.EqualFold(strings.TrimSpace(in.SkillID), "skill.thirdparty.prompt-template")
}

func (e *promptTemplateExecutor) Execute(ctx context.Context, in ExecuteInput) (map[string]interface{}, error) {
	template := extractString(in.Payload, "template", "prompt_template")
	if strings.TrimSpace(template) == "" {
		return nil, fmt.Errorf("template is required for prompt-template skill")
	}

	vars := extractVariablesMap(in.Payload["variables"])
	rendered := promptTemplateVarPattern.ReplaceAllStringFunc(template, func(token string) string {
		match := promptTemplateVarPattern.FindStringSubmatch(token)
		if len(match) != 2 {
			return token
		}
		key := strings.TrimSpace(match[1])
		if key == "" {
			return token
		}
		value, ok := vars[key]
		if !ok {
			return token
		}
		return fmt.Sprintf("%v", value)
	})

	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return map[string]interface{}{
		"skill_id":        in.SkillID,
		"version":         in.Version,
		"entrypoint":      in.Entrypoint,
		"rendered_text":   rendered,
		"variables_used":  len(vars),
		"variable_keys":   keys,
		"executor":        "prompt-template",
		"fallback_used":   false,
		"trace_id":        in.TraceID,
		"manifest_schema": extractString(in.Manifest, "schema"),
	}, nil
}

func extractVariablesMap(raw interface{}) map[string]interface{} {
	if raw == nil {
		return map[string]interface{}{}
	}
	switch v := raw.(type) {
	case map[string]interface{}:
		return v
	case map[string]string:
		out := make(map[string]interface{}, len(v))
		for k, item := range v {
			out[k] = item
		}
		return out
	default:
		return map[string]interface{}{}
	}
}
