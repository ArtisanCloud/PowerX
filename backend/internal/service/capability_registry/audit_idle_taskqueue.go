package capability_registry

import (
	"reflect"

	"github.com/google/uuid"
)

// idleTaskQueueObservation recognizes only the executed Core taskqueue.v2
// dequeue result. Caller flags, stored snapshots and unknown result shapes are
// never proof of an empty read. Authorization and live grants remain enforced
// by RuntimeInvoker before execution; this function does not grant access.
// Keep these fixed identifiers aligned with task_queue/runtime_contract.go;
// importing task_queue here would create a service dependency cycle.
func idleTaskQueueObservation(input InvocationAuditInput) bool {
	if input.PluginID != "corex.platform" ||
		input.CapabilityID != "com.corex.runtime.taskqueue.service_consume" ||
		input.ProtocolUsed != "core_internal" || input.Status != "completed" ||
		input.FallbackUsed || input.ErrorSummary != "" || input.IdempotencyKey != "" {
		return false
	}
	tenant, err := uuid.Parse(input.TenantUUID)
	if err != nil || tenant == uuid.Nil || tenant.String() != input.TenantUUID {
		return false
	}
	if len(input.RequestPayload) != 1 || len(input.ResponsePayload) != 2 {
		return false
	}
	body := input.RequestPayload["body"]
	if auditString(auditField(body, "contract_version")) != "taskqueue.v2" ||
		auditString(auditField(body, "operation")) != "dequeue" ||
		auditString(auditField(body, "tenant_key")) != input.TenantUUID ||
		auditString(input.ResponsePayload["contract_version"]) != "taskqueue.v2" {
		return false
	}
	messages := reflect.ValueOf(input.ResponsePayload["messages"])
	if !messages.IsValid() || messages.Kind() != reflect.Slice || messages.IsNil() || messages.Len() != 0 {
		return false
	}
	// RuntimeService returns []RuntimeMessage; decoded JSON returns []any.
	// In particular []byte is a JSON string, not a message list.
	kind := messages.Type().Elem().Kind()
	return kind == reflect.Struct || kind == reflect.Interface
}
