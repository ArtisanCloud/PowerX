package eventfabric

// TopicLifecycle 标识主题的生命周期状态，遵循规格中的 active/deprecated/retired 等阶段。
type TopicLifecycle string

const (
	// TopicLifecycleActive 表示主题可正常使用。
	TopicLifecycleActive TopicLifecycle = "active"
	// TopicLifecycleDeprecated 表示主题已进入下线倒计时，仍允许订阅但会触发告警。
	TopicLifecycleDeprecated TopicLifecycle = "deprecated"
	// TopicLifecycleRetired 表示主题已彻底下线，不再接受发布或订阅。
	TopicLifecycleRetired TopicLifecycle = "retired"
)

// TenantScopedModel 为事件骨干中所有租户隔离表提供统一字段。
type TenantScopedModel struct {
	TenantID string `json:"tenant_id"`
}

// AuditTrail 统一审计字段，鼓励后续模型嵌入使用。
type AuditTrail struct {
	CreatedBy string `json:"created_by"`
	UpdatedBy string `json:"updated_by"`
}
