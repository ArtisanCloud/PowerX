package plugin_identity

import (
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

const (
	CapabilityID   = "com.corex.iam.identity_context.service_read"
	Endpoint       = "core://iam/identity-context"
	Header         = "X-PowerX-Identity-Context"
	QuerySchema    = "powerx.iam.plugin-request-identity-query/v1"
	ResultSchema   = "powerx.iam.plugin-request-identity/v1"
	APIKeyScope    = "_scope.iam.identity_context.service_read"
	WSQuerySchema  = "powerx.iam.plugin-ws-identity-query/v1"
	WSResultSchema = "powerx.iam.plugin-ws-identity/v1"
)

type Query struct {
	Schema        string `json:"schema"`
	ContextTicket string `json:"context_ticket"`
}
type Operation struct {
	Operation string `json:"operation"`
	Request   Query  `json:"request"`
}
type WSQuery struct {
	Schema string `json:"schema"`
	Handle string `json:"ws_revalidation_handle"`
}
type WSOperation struct {
	Operation string  `json:"operation"`
	Request   WSQuery `json:"request"`
}
type IssueInput struct {
	PluginID           string
	Claims             reqctx.CoreXClaims
	Method             string
	Path               string
	Transport          string
	RequestID          string
	TraceID            string
	RequiredPermission string
}
type Result struct {
	Schema                   string   `json:"schema"`
	Mode                     string   `json:"mode"`
	PluginID                 string   `json:"plugin_id"`
	TenantUUID               string   `json:"tenant_uuid"`
	UserUUID                 string   `json:"user_uuid"`
	MemberUUID               *string  `json:"member_uuid"`
	IsRoot                   bool     `json:"is_root"`
	Roles                    []string `json:"roles"`
	Permissions              []string `json:"permissions"`
	PolicyVersion            string   `json:"policy_version"`
	PermsHash                string   `json:"perms_hash"`
	SourceMethod             string   `json:"source_method"`
	SourcePath               string   `json:"source_path"`
	SourceRequiredPermission string   `json:"source_required_permission"`
	Transport                string   `json:"transport"`
	SourceRequestID          string   `json:"source_request_id"`
	SourceTraceID            string   `json:"source_trace_id"`
	TraceID                  string   `json:"trace_id"`
	ProofExpiresAt           string   `json:"proof_expires_at"`
	UserSessionExpiresAt     string   `json:"user_session_expires_at"`
	WSRevalidationHandle     string   `json:"ws_revalidation_handle,omitempty"`
	WSRevalidationExpiresAt  string   `json:"ws_revalidation_expires_at,omitempty"`
}

// Codes deliberately identify the failed credential plane. Callers must not
// sign out the human user when the plugin service credential is rejected.
func failure(status int, code string) error {
	return dto.NewErrorWithCode(status, code, code, nil)
}
