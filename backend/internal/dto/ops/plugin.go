package ops

// PluginLifecycleActionRequest 为插件生命周期动作请求 DTO 骨架。
type PluginLifecycleActionRequest struct {
	PluginID   string `json:"plugin_id"`
	FromVer    string `json:"from_version,omitempty"`
	ToVer      string `json:"to_version,omitempty"`
	Action     string `json:"action"`
	Reason     string `json:"reason,omitempty"`
	Operator   string `json:"operator,omitempty"`
	ApprovalID string `json:"approval_id,omitempty"`
}
