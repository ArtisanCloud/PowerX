package capability_registry

const InvocationAuditClass = "capability_invocation_audit"
const InvocationNotificationClass = "capability_invocation_notification"

func TerminalInvocation(status string) bool {
	return status == "completed" || status == "success" || status == "succeeded" || status == "failed" || status == "error" || status == "denied"
}
