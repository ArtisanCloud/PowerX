package workflow

import (
	"fmt"
	"strings"
)

type decisionExecutor struct{}

func (d *decisionExecutor) Type() string {
	return "decision"
}

func (d *decisionExecutor) SubjectType() string {
	return "system"
}

func (d *decisionExecutor) Validate(step StepDefinition) error {
	_, _, err := d.extractRoutes(step)
	return err
}

func (d *decisionExecutor) Next(step StepDefinition, result StepResult) ([]string, error) {
	routes, defaultRoute, err := d.extractRoutes(step)
	if err != nil {
		return nil, err
	}

	decisionKey := strings.TrimSpace(result.Decision)
	if decisionKey == "" && result.Output != nil {
		if raw, ok := result.Output["decision"]; ok {
			if s := asString(raw); s != "" {
				decisionKey = s
			}
		}
		if raw, ok := result.Output["route"]; ok && decisionKey == "" {
			if s := asString(raw); s != "" {
				decisionKey = s
			}
		}
	}

	if decisionKey != "" {
		if next, ok := routes[decisionKey]; ok {
			return cloneStrings(next), nil
		}
	}

	if len(defaultRoute) > 0 {
		return cloneStrings(defaultRoute), nil
	}

	if next, ok := routes["default"]; ok {
		return cloneStrings(next), nil
	}

	if len(step.NextStepIDs) > 0 {
		return cloneStrings(step.NextStepIDs), nil
	}
	return nil, fmt.Errorf("decision step %s produced no matching route", step.ID)
}

func (d *decisionExecutor) extractRoutes(step StepDefinition) (map[string][]string, []string, error) {
	if step.Config == nil {
		return nil, nil, fmt.Errorf("decision step %s requires config.routes", step.ID)
	}
	rawRoutes, ok := step.Config["routes"]
	if !ok {
		return nil, nil, fmt.Errorf("decision step %s requires config.routes", step.ID)
	}

	routes, err := parseRouteMap(rawRoutes)
	if err != nil {
		return nil, nil, fmt.Errorf("decision step %s: %w", step.ID, err)
	}
	if len(routes) == 0 {
		return nil, nil, fmt.Errorf("decision step %s has empty routes definition", step.ID)
	}

	var defaultRoute []string
	if rawDefault, ok := step.Config["default_route"]; ok {
		defaultRoute, err = asStringSlice(rawDefault)
		if err != nil {
			return nil, nil, fmt.Errorf("decision step %s default_route: %w", step.ID, err)
		}
		defaultRoute = normalizeStrings(defaultRoute)
	}

	return routes, defaultRoute, nil
}

func parseRouteMap(input any) (map[string][]string, error) {
	out := map[string][]string{}
	switch routes := input.(type) {
	case map[string]any:
		for key, val := range routes {
			next, err := asStringSlice(val)
			if err != nil {
				return nil, fmt.Errorf("route %s: %w", key, err)
			}
			next = normalizeStrings(next)
			if len(next) == 0 {
				return nil, fmt.Errorf("route %s has no targets", key)
			}
			out[strings.TrimSpace(key)] = next
		}
	case map[string]string:
		for key, target := range routes {
			target = strings.TrimSpace(target)
			if target == "" {
				return nil, fmt.Errorf("route %s has empty target", key)
			}
			out[strings.TrimSpace(key)] = []string{target}
		}
	case []any:
		for _, item := range routes {
			entry, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("route entry must be object")
			}
			name := strings.TrimSpace(asString(entry["name"]))
			if name == "" {
				return nil, fmt.Errorf("route entry missing name")
			}
			next, err := asStringSlice(entry["next"])
			if err != nil {
				return nil, fmt.Errorf("route %s: %w", name, err)
			}
			next = normalizeStrings(next)
			if len(next) == 0 {
				return nil, fmt.Errorf("route %s has no targets", name)
			}
			out[name] = next
		}
	default:
		return nil, fmt.Errorf("routes must be object or array, got %T", input)
	}
	return out, nil
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

func asStringSlice(value any) ([]string, error) {
	switch v := value.(type) {
	case nil:
		return nil, fmt.Errorf("value is nil")
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("value is empty")
		}
		return []string{strings.TrimSpace(v)}, nil
	case []string:
		return v, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s := asString(item)
			if s == "" {
				return nil, fmt.Errorf("contains non-string or empty value")
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected string or array of strings, got %T", value)
	}
}
