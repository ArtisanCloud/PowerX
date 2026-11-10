package capability_registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"

	auditpayload "github.com/ArtisanCloud/PowerX/pkg/corex/audit/payload"
)

// 审计投影不参与判权：只识别已经执行完成的正式类型化查询，未知格式走通用有界副本。
// 不依赖 authorization_baseline Service，避免反向依赖及再次读取权限数据。
func baselineAuditSummary(in InvocationAuditInput) (json.RawMessage, json.RawMessage, bool) {
	if in.CapabilityID != "com.corex.iam.authorization_baseline.service_read" || in.ProtocolUsed != "core_internal" || in.FallbackUsed || (in.Status != "completed" && in.Status != "success" && in.Status != "succeeded") {
		return nil, nil, false
	}
	body := auditField(in.RequestPayload, "body")
	if auditString(auditField(body, "operation")) != "read_member_baselines" {
		return nil, nil, false
	}
	query := auditField(body, "request")
	result := auditField(in.ResponsePayload, "result")
	if auditString(auditField(query, "schema")) != "powerx.iam.authorization-baseline-query/v1" || auditString(auditField(result, "schema")) != "powerx.iam.authorization-baseline-result/v1" || auditString(auditField(result, "tenant_uuid")) != in.TenantUUID {
		return nil, nil, false
	}
	members, permissions, items := auditValue(auditField(query, "member_uuids")), auditValue(auditField(query, "permission_codes")), auditValue(auditField(result, "items"))
	if !auditSequence(members, 100) || !auditSequence(permissions, 10) || !auditSequence(items, 1000) || items.Len() != members.Len()*permissions.Len() {
		return nil, nil, false
	}
	queryHash := sha256.New()
	decisionHash := sha256.New()
	for _, sequence := range []reflect.Value{members, permissions} {
		for i := 0; i < sequence.Len(); i++ {
			text := auditString(sequence.Index(i).Interface())
			if text == "" || len(text) > 256 {
				return nil, nil, false
			}
			queryHash.Write([]byte(text))
			queryHash.Write([]byte{0})
		}
		queryHash.Write([]byte{0})
	}
	allowed, denied, errors, unknown := 0, 0, 0, 0
	for i := 0; i < items.Len(); i++ {
		item := items.Index(i).Interface()
		status := auditString(auditField(item, "status"))
		decision := auditValue(auditField(item, "allowed"))
		switch {
		case status == "error":
			errors++
		case status == "decided" && decision.IsValid() && decision.Kind() == reflect.Bool:
			if decision.Bool() {
				allowed++
			} else {
				denied++
			}
		default:
			unknown++
		}
		for _, field := range []string{"member_uuid", "permission_code", "status", "error_code", "authorization_revision", "baseline_kind", "data_scope", "scope_fingerprint", "granted_at", "known_since", "visible_since", "valid_until", "recheck_at"} {
			text := auditString(auditField(item, field))
			if len(text) > 512 {
				return nil, nil, false
			}
			decisionHash.Write([]byte(text))
			decisionHash.Write([]byte{0})
		}
		if decision.IsValid() && decision.Kind() == reflect.Bool {
			if decision.Bool() {
				decisionHash.Write([]byte{1})
			} else {
				decisionHash.Write([]byte{2})
			}
		} else {
			decisionHash.Write([]byte{3})
		}
	}
	request := map[string]any{"operation": "read_member_baselines", "schema": "powerx.iam.authorization-baseline-query/v1", "member_count": members.Len(), "permission_count": permissions.Len(), "query_digest": hex.EncodeToString(queryHash.Sum(nil))}
	response := map[string]any{"schema": "powerx.iam.authorization-baseline-result/v1", "tenant_uuid": in.TenantUUID, "plugin_id": auditString(auditField(result, "plugin_id")), "evaluated_at": auditString(auditField(result, "evaluated_at")), "item_count": items.Len(), "allowed_count": allowed, "denied_count": denied, "error_count": errors, "unknown_count": unknown, "decision_digest": hex.EncodeToString(decisionHash.Sum(nil)), "detail_omitted": true}
	return auditpayload.Summary(request), auditpayload.Summary(response), true
}

func auditValue(v any) reflect.Value {
	r := reflect.ValueOf(v)
	for depth := 0; r.IsValid() && (r.Kind() == reflect.Pointer || r.Kind() == reflect.Interface); depth++ {
		if depth > 4 || r.IsNil() {
			return reflect.Value{}
		}
		r = r.Elem()
	}
	return r
}
func auditField(v any, key string) any {
	r := auditValue(v)
	if !r.IsValid() {
		return nil
	}
	if r.Kind() == reflect.Map && r.Type().Key().Kind() == reflect.String {
		value := r.MapIndex(reflect.ValueOf(key).Convert(r.Type().Key()))
		if value.IsValid() {
			return value.Interface()
		}
		return nil
	}
	if r.Kind() == reflect.Struct {
		for i := 0; i < r.NumField(); i++ {
			field := r.Type().Field(i)
			if field.PkgPath == "" && (field.Tag.Get("json") == key || len(field.Tag.Get("json")) > len(key) && field.Tag.Get("json")[:len(key)+1] == key+",") {
				return r.Field(i).Interface()
			}
		}
	}
	return nil
}
func auditString(v any) string {
	r := auditValue(v)
	if r.IsValid() && r.Kind() == reflect.String {
		return r.String()
	}
	return ""
}
func auditSequence(v reflect.Value, max int) bool {
	return v.IsValid() && (v.Kind() == reflect.Slice || v.Kind() == reflect.Array) && v.Len() > 0 && v.Len() <= max
}
