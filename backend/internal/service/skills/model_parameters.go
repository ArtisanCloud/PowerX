package skills

import "fmt"

// Explicit per-definition parameters; never choose a business-specific model path.
func ManifestLLMParameters(policy map[string]any) (map[string]any, error) {
	out := map[string]any{}
	raw, exists := policy["parameters"]
	if !exists {
		return out, nil
	}
	values, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("skill.model_parameters_invalid")
	}
	for key, value := range values {
		switch key {
		case "thinking":
			if _, ok := value.(bool); !ok {
				return nil, fmt.Errorf("skill.model_parameters_invalid: %s", key)
			}
		case "max_tokens":
			var n float64
			switch v := value.(type) {
			case int:
				n = float64(v)
			case float64:
				n = v
			default:
				return nil, fmt.Errorf("skill.model_parameters_invalid: %s", key)
			}
			if key == "max_tokens" && (n < 1 || n > 16384 || n != float64(int(n))) {
				return nil, fmt.Errorf("skill.model_parameters_invalid: %s", key)
			}
		default:
			return nil, fmt.Errorf("skill.model_parameter_unsupported: %s", key)
		}
		out[key] = value
	}
	return out, nil
}
