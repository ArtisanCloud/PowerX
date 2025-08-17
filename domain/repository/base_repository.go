package repository

import (
	"context"

	"gorm.io/gorm"
)

// BaseRepository 基础仓储接口
// 定义了通用的数据访问操作方法
type BaseRepository[T any] interface {
	// Create 创建单个实体
	Create(ctx context.Context, entity *T) error

	// CreateInBatches 批量创建实体
	CreateInBatches(ctx context.Context, entities []*T, batchSize int) error

	// Update 更新实体
	Update(ctx context.Context, entity *T) error

	// UpdateByID 根据ID更新实体
	UpdateByID(ctx context.Context, id interface{}, updates map[string]interface{}) error

	// Delete 删除实体
	Delete(ctx context.Context, entity *T) error

	// DeleteByID 根据ID删除实体
	DeleteByID(ctx context.Context, id interface{}) error

	// DeleteByIDs 根据ID列表批量删除实体
	DeleteByIDs(ctx context.Context, ids []interface{}) error

	// FindByID 根据ID查找实体
	FindByID(ctx context.Context, id interface{}) (*T, error)

	// FindByIDs 根据ID列表查找实体
	FindByIDs(ctx context.Context, ids []interface{}) ([]*T, error)

	// FindOne 根据条件查找单个实体
	FindOne(ctx context.Context, condition interface{}, args ...interface{}) (*T, error)

	// FindAll 查找所有实体
	FindAll(ctx context.Context) ([]*T, error)

	// FindByCondition 根据条件查找实体列表
	FindByCondition(ctx context.Context, condition interface{}, args ...interface{}) ([]*T, error)

	// Count 统计实体数量
	Count(ctx context.Context) (int64, error)

	// CountByCondition 根据条件统计实体数量
	CountByCondition(ctx context.Context, condition interface{}, args ...interface{}) (int64, error)

	// Exists 检查实体是否存在
	Exists(ctx context.Context, condition interface{}, args ...interface{}) (bool, error)

	// ExistsByID 根据ID检查实体是否存在
	ExistsByID(ctx context.Context, id interface{}) (bool, error)

	// GetDB 获取数据库连接
	GetDB() *gorm.DB

	// WithTx 在事务中执行操作
	WithTx(tx *gorm.DB) BaseRepository[T]

	// BeginTx 开始事务
	BeginTx(ctx context.Context) (*gorm.DB, error)

	// CommitTx 提交事务
	CommitTx(tx *gorm.DB) error

	// RollbackTx 回滚事务
	RollbackTx(tx *gorm.DB) error
}

// PageRepository 分页查询接口
// 扩展基础仓储接口，提供分页查询功能
type PageRepository[T any] interface {
	BaseRepository[T]

	// FindWithPagination 分页查询实体
	// page: 页码（从1开始）
	// pageSize: 每页大小
	// condition: 查询条件
	// args: 查询参数
	FindWithPagination(
		ctx context.Context,
		page, pageSize int,
		condition interface{},
		args ...interface{},
	) (*Page[*T], error)

	// FindWithPaginationAndSort 分页查询实体（带排序）
	// page: 页码（从1开始）
	// pageSize: 每页大小
	// orderBy: 排序字段
	// condition: 查询条件
	// args: 查询参数
	FindWithPaginationAndSort(
		ctx context.Context,
		page, pageSize int,
		orderBy string,
		condition interface{},
		args ...interface{},
	) (*Page[*T], error)
}

// Page 分页结果结构
type Page[T any] struct {
	List       []T   `json:"list"`        // 数据列表
	Total      int64 `json:"total"`       // 总记录数
	Page       int   `json:"page"`        // 当前页码
	PageSize   int   `json:"page_size"`   // 每页大小
	TotalPages int   `json:"total_pages"` // 总页数
	HasNext    bool  `json:"has_next"`    // 是否有下一页
	HasPrev    bool  `json:"has_prev"`    // 是否有上一页
}

// DebugKey 调试模式的上下文键
type DebugKey struct{}

// String 实现 Stringer 接口
func (d DebugKey) String() string {
	return "debug"
}
