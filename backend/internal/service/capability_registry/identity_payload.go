package capability_registry

import (
	"bytes"
	"encoding/json"
)

// Only persistence/audit copies are redacted; the typed execution payload
// remains intact. Never retain an identity capability's bearer proof in traces.
func redactIdentityTicketPayload(capabilityID string, payload map[string]interface{}) map[string]interface{} {
	if capabilityID != "com.corex.iam.identity_context.service_read" {
		return payload
	}
	if payload == nil {
		return nil
	}
	// typed invoker 的 Result 可能仍是 Go struct；先转为仅用于审计的 JSON
	// 副本，再递归脱敏，否则 response.result 内的 WS handle 会绕过 map 检查。
	raw, err := json.Marshal(payload)
	if err != nil {
		return map[string]interface{}{"identity_payload_redacted": true}
	}
	var copy map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&copy) != nil {
		return map[string]interface{}{"identity_payload_redacted": true}
	}
	var redact func(any) any
	redact = func(value any) any {
		switch v := value.(type) {
		case map[string]interface{}:
			out := map[string]interface{}{}
			for k, x := range v {
				if k == "context_ticket" || k == "ws_revalidation_handle" {
					out[k] = "[REDACTED]"
				} else {
					out[k] = redact(x)
				}
			}
			return out
		case []interface{}:
			out := make([]interface{}, len(v))
			for i, x := range v {
				out[i] = redact(x)
			}
			return out
		default:
			return v
		}
	}
	return redact(copy).(map[string]interface{})
}
