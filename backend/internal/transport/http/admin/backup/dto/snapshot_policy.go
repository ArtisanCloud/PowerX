package dto

// SnapshotPolicyRequest 只接收用户可编辑配置，不接受默认选择、路径或调度锚点。
type SnapshotPolicyRequest struct {
	Name            string `json:"name"`
	Enabled         bool   `json:"enabled"`
	IntervalMinutes int    `json:"interval_minutes"`
	RetentionDays   int    `json:"retention_days"`
	RetentionCount  int    `json:"retention_count"`
}

type SnapshotPolicyStatusRequest struct {
	Enabled     *bool `json:"enabled"`
	MakeDefault bool  `json:"make_default,omitempty"`
}
