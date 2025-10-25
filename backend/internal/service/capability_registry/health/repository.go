package health

import router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"

// MemoryRepository 兼容旧引用，实际实现在 router 包内。
type MemoryRepository = router.MemoryRepository

// NewMemoryRepository 返回 Router 包的默认内存实现。
func NewMemoryRepository() *MemoryRepository {
	return router.NewMemoryRepository()
}
