package authorization_baseline

import "encoding/json"

const (
	CapabilityID   = "com.corex.iam.authorization_baseline.service_read"
	Endpoint       = "core://iam/authorization-baseline"
	Operation      = "read_member_baselines"
	QuerySchema    = "powerx.iam.authorization-baseline-query/v1"
	ResultSchema   = "powerx.iam.authorization-baseline-result/v1"
	APIKeyScope    = "_scope.iam.authorization_baseline.service_read"
	APIKeyResource = "iam_authorization_baseline_read"
)

type Query struct {
	Schema          string   `json:"schema"`
	MemberUUIDs     []string `json:"member_uuids"`
	PermissionCodes []string `json:"permission_codes"`
}
type Result struct {
	Schema      string `json:"schema"`
	TenantUUID  string `json:"tenant_uuid"`
	PluginID    string `json:"plugin_id"`
	EvaluatedAt string `json:"evaluated_at"`
	Items       []Item `json:"items"`
}
type Item struct {
	MemberUUID            string  `json:"member_uuid"`
	PermissionCode        string  `json:"permission_code"`
	Status                string  `json:"status"`
	ErrorCode             string  `json:"error_code,omitempty"`
	Allowed               *bool   `json:"allowed,omitempty"`
	BaselineKind          string  `json:"baseline_kind,omitempty"`
	GrantedAt             *string `json:"granted_at"`
	KnownSince            *string `json:"known_since,omitempty"`
	VisibleSince          *string `json:"visible_since"`
	AuthorizationRevision string  `json:"authorization_revision,omitempty"`
	DataScope             string  `json:"data_scope,omitempty"`
	ScopeFingerprint      string  `json:"scope_fingerprint,omitempty"`
	ValidUntil            *string `json:"valid_until"`
	RecheckAt             *string `json:"recheck_at"`
}

// MarshalJSON 保证错误分支不能因 bool 零值而伪装成业务拒绝。
func (i Item) MarshalJSON() ([]byte, error) {
	if i.Status == "error" {
		return json.Marshal(struct {
			MemberUUID     string `json:"member_uuid"`
			PermissionCode string `json:"permission_code"`
			Status         string `json:"status"`
			ErrorCode      string `json:"error_code"`
		}{i.MemberUUID, i.PermissionCode, i.Status, i.ErrorCode})
	}
	type decided Item
	return json.Marshal(decided(i))
}
