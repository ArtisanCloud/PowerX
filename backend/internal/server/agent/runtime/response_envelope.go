package runtime

import "github.com/ArtisanCloud/PowerX/pkg/corex/agent/evidence"

const AgentResponseEnvelopeSchema = evidence.ReportSchema

func ValidateAgentResponseEnvelope(value any) (map[string]any, error) {
	if m, ok := value.(map[string]any); ok && m["schema"] == evidence.ReportSchemaV5 {
		return evidence.ValidateReportV5(value)
	}
	return evidence.ValidateReport(value)
}

func validateAgentResponseEnvelope(value any, _ map[string]struct{}) (map[string]any, error) {
	return ValidateAgentResponseEnvelope(value)
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
	return responseEnvelopeFromExecutionResultWithTaskRefs(data, nil)
}

func finalResponseCompanion(data map[string]any, key string) any {
	if nested, ok := data["result"].(map[string]any); ok {
		return nested[key]
	}
	return data[key]
}

func responseEnvelopeFromExecutionResultWithTaskRefs(data map[string]any, upstreamTaskRefs map[string]struct{}) (map[string]any, error) {
	if data == nil {
		return nil, nil
	}
	if result, ok := data["result"].(map[string]any); ok {
		if raw, ok := result["response_envelope"]; ok {
			return validateAgentResponseEnvelope(raw, upstreamTaskRefs)
		}
		return nil, nil
	}
	if raw, ok := data["response_envelope"]; ok {
		return validateAgentResponseEnvelope(raw, upstreamTaskRefs)
	}
	return nil, nil
}
