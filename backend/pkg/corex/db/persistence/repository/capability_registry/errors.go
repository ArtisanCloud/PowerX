package capability_registry

import "errors"

var (
	// ErrNotFound 表示未找到指定能力注册快照。
	ErrNotFound = errors.New("capability registry: registration not found")

	// ErrVersionConflict 表示版本不匹配，触发乐观锁冲突。
	ErrVersionConflict = errors.New("capability registry: version conflict")

	// ErrDuplicateAdapterID 表示相同版本中存在重复的适配器 ID。
	ErrDuplicateAdapterID = errors.New("capability registry: duplicate adapter id")

	// ErrCapabilityRecordNotFound 表示不存在指定的 CapabilityRecord。
	ErrCapabilityRecordNotFound = errors.New("capability registry: capability record not found")

	// ErrWorkflowTemplateNotFound 表示不存在指定的 WorkflowTemplateRef。
	ErrWorkflowTemplateNotFound = errors.New("capability registry: workflow template not found")

	// ErrSyncJobNotFound 表示不存在指定的同步任务。
	ErrSyncJobNotFound = errors.New("capability registry: capability sync job not found")

	// ErrInvocationTraceNotFound 表示不存在指定的调用追踪记录。
	ErrInvocationTraceNotFound = errors.New("capability registry: invocation trace not found")

	// ErrEventPublicationNotFound 表示不存在指定的事件投递记录。
	ErrEventPublicationNotFound = errors.New("capability registry: event publication not found")
)
