package runtime

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// AgentResponseEnvelopeSchema is the single platform-owned final-response
// contract. Skills provide only business data; PowerX derives the conclusion,
// acceptance list, and rendered layout.
const AgentResponseEnvelopeSchema = "powerx.agent.response/v3"

func ValidateAgentResponseEnvelope(value any) (map[string]any, error) {
	envelope, ok := value.(map[string]any)
	if !ok || len(envelope) == 0 {
		return nil, fmt.Errorf("agent.response_contract_invalid: response_envelope is required")
	}
	if strings.TrimSpace(anyToString(envelope["schema"])) != AgentResponseEnvelopeSchema {
		return nil, fmt.Errorf("agent.response_contract_invalid: schema must be %s", AgentResponseEnvelopeSchema)
	}
	outcome := strings.TrimSpace(anyToString(envelope["outcome"]))
	switch outcome {
	case "completed", "needs_action", "blocked", "failed":
	default:
		return nil, fmt.Errorf("agent.response_contract_invalid: invalid outcome")
	}
	if strings.TrimSpace(anyToString(envelope["kind"])) == "" {
		return nil, fmt.Errorf("agent.response_contract_invalid: kind is required")
	}
	presentation, ok := envelope["presentation"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("agent.response_contract_invalid: presentation is required")
	}
	if err := rejectUnknownResponseFields(envelope, map[string]bool{"schema": true, "kind": true, "outcome": true, "presentation": true}, "response_envelope"); err != nil {
		return nil, err
	}
	if err := rejectUnknownResponseFields(presentation, map[string]bool{"facts": true, "metrics": true, "hypotheses": true, "gaps": true, "actions": true}, "presentation"); err != nil {
		return nil, err
	}
	facts, err := requiredResponseArray(presentation, "facts")
	if err != nil {
		return nil, err
	}
	metrics, err := requiredResponseArray(presentation, "metrics")
	if err != nil {
		return nil, err
	}
	hypotheses, err := requiredResponseStrings(presentation, "hypotheses")
	if err != nil {
		return nil, err
	}
	gaps, err := requiredResponseStrings(presentation, "gaps")
	if err != nil {
		return nil, err
	}
	actions, err := requiredResponseStrings(presentation, "actions")
	if err != nil {
		return nil, err
	}
	if outcome == "completed" && len(facts) == 0 && len(metrics) == 0 {
		return nil, fmt.Errorf("agent.response_contract_invalid: completed requires facts or metrics")
	}
	if outcome == "completed" && (len(hypotheses) > 0 || len(gaps) > 0) {
		return nil, fmt.Errorf("agent.response_contract_invalid: completed cannot contain hypotheses or gaps")
	}
	for _, item := range facts {
		record, ok := item.(map[string]any)
		if ok {
			if err := rejectUnknownResponseFields(record, map[string]bool{"statement": true, "source": true}, "fact"); err != nil {
				return nil, err
			}
		}
		if !ok || strings.TrimSpace(anyToString(record["statement"])) == "" || !validResponseSource(record["source"]) {
			return nil, fmt.Errorf("agent.response_contract_invalid: fact requires statement and typed source")
		}
	}
	for _, item := range metrics {
		record, ok := item.(map[string]any)
		if ok {
			if err := rejectUnknownResponseFields(record, map[string]bool{"label": true, "numerator": true, "denominator": true, "formula": true, "display_value": true, "source": true}, "metric"); err != nil {
				return nil, err
			}
		}
		if !ok || strings.TrimSpace(anyToString(record["label"])) == "" || strings.TrimSpace(anyToString(record["formula"])) == "" || strings.TrimSpace(anyToString(record["display_value"])) == "" || !hasNumericValue(record["numerator"]) || !hasNumericValue(record["denominator"]) || !validResponseSource(record["source"]) {
			return nil, fmt.Errorf("agent.response_contract_invalid: metric requires label, numeric numerator and denominator, formula, display_value and typed source")
		}
		if err := validateMetricValue(record); err != nil {
			return nil, err
		}
	}
	if len(actions) == 0 && outcome == "needs_action" {
		return nil, fmt.Errorf("agent.response_contract_invalid: needs_action requires actions")
	}
	return envelope, nil
}

func hasNumericValue(value any) bool {
	_, ok := numericValue(value)
	return ok
}

func rejectUnknownResponseFields(value map[string]any, allowed map[string]bool, scope string) error {
	for key := range value {
		if !allowed[key] {
			return fmt.Errorf("agent.response_contract_invalid: %s contains unsupported field %q", scope, key)
		}
	}
	return nil
}

func requiredResponseArray(value map[string]any, field string) ([]any, error) {
	raw, exists := value[field]
	if !exists {
		return nil, fmt.Errorf("agent.response_contract_invalid: presentation.%s is required", field)
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("agent.response_contract_invalid: presentation.%s must be an array", field)
	}
	return items, nil
}

func requiredResponseStrings(value map[string]any, field string) ([]string, error) {
	items, err := requiredResponseArray(value, field)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("agent.response_contract_invalid: presentation.%s must contain non-empty strings", field)
		}
		out = append(out, strings.TrimSpace(text))
	}
	return out, nil
}

func validResponseSource(value any) bool {
	source, ok := value.(map[string]any)
	if !ok {
		return false
	}
	if err := rejectUnknownResponseFields(source, map[string]bool{"type": true, "ref": true}, "source"); err != nil {
		return false
	}
	typeName := strings.TrimSpace(anyToString(source["type"]))
	ref := strings.TrimSpace(anyToString(source["ref"]))
	return (typeName == "input" || typeName == "task") && ref != ""
}

func validateMetricValue(metric map[string]any) error {
	numerator, numeratorOK := numericValue(metric["numerator"])
	denominator, denominatorOK := numericValue(metric["denominator"])
	display := strings.TrimSpace(anyToString(metric["display_value"]))
	if !numeratorOK || !denominatorOK || denominator == 0 || !strings.HasSuffix(display, "%") {
		return nil
	}
	actual, ok := numericValue(strings.TrimSuffix(display, "%"))
	if !ok {
		return fmt.Errorf("agent.response_contract_invalid: metric display_value is not numeric")
	}
	if math.Abs(actual-(numerator/denominator*100)) > 0.06 {
		return fmt.Errorf("agent.response_contract_invalid: metric display_value does not match numerator and denominator")
	}
	return nil
}

func numericValue(value any) (float64, bool) {
	text := strings.ReplaceAll(strings.TrimSpace(anyToString(value)), ",", "")
	if text == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(text, 64)
	return parsed, err == nil
}

func responseEnvelopeFromResult(out map[string]any) (map[string]any, error) {
	if out == nil {
		return nil, nil
	}
	if raw, ok := out["response_envelope"]; ok {
		return ValidateAgentResponseEnvelope(raw)
	}
	return nil, nil
}

func responseEnvelopeFromExecutionResult(data map[string]any) (map[string]any, error) {
	if data == nil {
		return nil, nil
	}
	if result, ok := data["result"].(map[string]any); ok {
		return responseEnvelopeFromResult(result)
	}
	return responseEnvelopeFromResult(data)
}
