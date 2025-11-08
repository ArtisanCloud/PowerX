package capability_registry

import "errors"

var (
	// ErrNotFound 表示未找到指定能力注册快照。
	ErrNotFound = errors.New("capability registry: registration not found")

	// ErrVersionConflict 表示版本不匹配，触发乐观锁冲突。
	ErrVersionConflict = errors.New("capability registry: version conflict")

	// ErrDuplicateAdapterID 表示相同版本中存在重复的适配器 ID。
	ErrDuplicateAdapterID = errors.New("capability registry: duplicate adapter id")
)
