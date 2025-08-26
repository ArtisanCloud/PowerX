package repository

import (
	"context"
	"errors"
	"github.com/ArtisanCloud/PowerX/domain/model"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

// BaseRepository 提供通用的 CRUD 操作
type BaseRepository[T any] struct {
	DB *gorm.DB
}

// NewBaseRepository 创建新的 BaseRepository 实例
func NewBaseRepository[T any](db *gorm.DB) *BaseRepository[T] {
	return &BaseRepository[T]{DB: db}
}

func (r *BaseRepository[T]) WithDB(db *gorm.DB) *BaseRepository[T] {
	r.DB = db
	return r
}

// CreateBatch 批量创建记录
func (r *BaseRepository[T]) CreateBatch(ctx context.Context, objs []*T) ([]*T, error) {
	if len(objs) == 0 {
		return nil, nil
	}

	query := r.DB.WithContext(ctx)

	debug, ok := ctx.Value(utils.DebugKey).(bool)
	if ok && debug {
		query = query.Debug()
	}

	result := query.Create(&objs)
	if result.Error != nil {
		logger.Error(ctx, result.Error.Error()) // ✅ 已替换
		//if strings.Contains(result.Error.Error(), "duplicated") {
		//	return nil, errors.New("关键名称信息不能重复")
		//}
		//return nil, errors.New("inner db error, pls check the log")
		return nil, result.Error
	}

	return objs, nil
}

// Create 创建新记录，并返回创建后的对象
func (r *BaseRepository[T]) Create(ctx context.Context, obj *T) (*T, error) {
	query := r.DB.WithContext(ctx)

	debug, ok := ctx.Value(utils.DebugKey).(bool)
	if ok && debug {
		query = query.Debug()
	}

	result := query.Create(obj)
	if result.Error != nil {
		//logger.Error(ctx, result.Error.Error()) // ✅ 已替换
		//if strings.Contains(result.Error.Error(), "duplicated") {
		//	return nil, errors.New("关键名称信息不能重复")
		//}
		//return nil, errors.New("inner db error, pls check the log")
		return nil, result.Error
	}
	return obj, nil
}

func getUpdatableColumns[T any](db *gorm.DB) []string {
	var entity T
	stmt := &gorm.Statement{DB: db}
	_ = stmt.Parse(&entity)

	var columns []string
	for _, field := range stmt.Schema.Fields {
		if field.PrimaryKey || field.DBName == "" {
			continue
		}
		// 根据你业务判断是否排除这些字段：
		if field.Name == "created_at" || field.Name == "updated_at" {
			continue
		}
		columns = append(columns, field.DBName)
	}
	return columns
}

// Upsert 插入或更新单个记录，并返回执行后的对象
func (r *BaseRepository[T]) Upsert(ctx context.Context, obj *T, uniqueFields []clause.Column) (*T, error) {
	query := r.DB.WithContext(ctx)

	debug, ok := ctx.Value(utils.DebugKey).(bool)
	if ok && debug {
		query = query.Debug()
	}

	result := query.Clauses(clause.OnConflict{
		Columns:   uniqueFields,
		DoUpdates: clause.AssignmentColumns(getUpdatableColumns[T](r.DB)),
	}).Create(obj)

	if result.Error != nil {
		return nil, result.Error
	}

	return obj, nil
}

// UpsertBatch 批量插入或更新记录，并返回执行后的对象列表
func (r *BaseRepository[T]) UpsertBatch(ctx context.Context, objs []*T, uniqueFields []clause.Column) ([]*T, error) {
	tx := r.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	debug, ok := ctx.Value(utils.DebugKey).(bool)
	if ok && debug {
		tx = tx.Debug()
	}

	result := tx.Clauses(clause.OnConflict{
		Columns:   uniqueFields,
		DoUpdates: clause.AssignmentColumns(getUpdatableColumns[T](r.DB)),
	}).Create(objs)

	if result.Error != nil {
		tx.Rollback()
		return nil, result.Error
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	return objs, nil
}

// Update 更新记录，并返回更新后的对象
func (r *BaseRepository[T]) Update(ctx context.Context, obj *T) (*T, error) {
	query := r.DB.WithContext(ctx)

	debug, ok := ctx.Value(utils.DebugKey).(bool)
	if ok && debug {
		query = query.Debug()
	}

	result := query.Save(obj)
	if result.RowsAffected == 0 {
		return nil, errors.New("record not found")
	}
	return obj, result.Error
}

func (r *BaseRepository[T]) UpdateByID(
	ctx context.Context,
	id uint64,
	fields map[string]interface{},
	callback func(*gorm.DB) *gorm.DB,
) (*T, error) {

	if id == 0 {
		return nil, errors.New("invalid id")
	}
	if len(fields) == 0 {
		return nil, errors.New("no fields to update")
	}

	// 允许更新的字段白名单（去除主键、created_at/updated_at 等）
	allowCols := map[string]struct{}{}
	for _, c := range getUpdatableColumns[T](r.DB) {
		allowCols[strings.ToLower(c)] = struct{}{}
	}

	// 过滤不可更新字段（保留 map 中与白名单交集的键）
	safeFields := make(map[string]interface{}, len(fields))
	for k, v := range fields {
		if _, ok := allowCols[strings.ToLower(k)]; ok {
			safeFields[k] = v
		}
	}
	if len(safeFields) == 0 {
		return nil, errors.New("no valid fields to update")
	}

	// 执行更新
	var mdl T
	query := r.DB.WithContext(ctx).Model(&mdl).Where("id = ?", id)

	if debug, ok := ctx.Value(utils.DebugKey).(bool); ok && debug {
		query = query.Debug()
	}
	if callback != nil {
		query = callback(query)
	}

	res := query.Updates(safeFields)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, errors.New("record not found")
	}

	// 读取并返回更新后的对象（复用已有的 GetById 逻辑和 callback）
	return r.GetById(ctx, id, callback)
}

// Patch 部分更新记录
func (r *BaseRepository[T]) Patch(ctx context.Context, where map[string]interface{}, fields map[string]interface{}) (*T, error) {
	var obj T
	query := r.DB.WithContext(ctx).Model(&obj)

	debug, ok := ctx.Value(utils.DebugKey).(bool)
	if ok && debug {
		query = query.Debug()
	}

	for key, value := range where {
		query = query.Where(key+" = ?", value)
	}

	result := query.Updates(fields)
	if result.RowsAffected == 0 {
		return nil, errors.New("record not found")
	}
	return &obj, result.Error
}

// Delete 删除记录
func (r *BaseRepository[T]) Delete(ctx context.Context, where map[string]interface{}, obj *T, softDelete bool) (*T, error) {
	var mdl T
	query := r.DB.WithContext(ctx).Model(&mdl)

	if debug, ok := ctx.Value(utils.DebugKey).(bool); ok && debug {
		query = query.Debug()
	}

	// 分支一：按 where 条件删除（推荐用于批量/覆盖式写入前的清理）
	if where != nil {
		for key, value := range where {
			if strings.Contains(key, "?") {
				query = query.Where(key, value)
			} else {
				query = query.Where(key+" = ?", value)
			}
		}
		if !softDelete {
			query = query.Unscoped()
		}
		result := query.Delete(&mdl)
		// 数据库错误才返回；0 行删除视为幂等成功
		if result.Error != nil {
			return nil, result.Error
		}
		return nil, nil
	}

	// 分支二：按主键对象删除（期望严格命中 1 行）
	if obj != nil {
		if !softDelete {
			query = query.Unscoped()
		}
		result := query.Delete(obj)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 0 {
			return nil, errors.New("record not found")
		}
		return obj, nil
	}

	return nil, errors.New("no delete condition provided")
}

// FindByCondition 查询并返回分页结果
func (r *BaseRepository[T]) FindByCondition(
	ctx context.Context,
	conditions map[string]interface{},
	page, pageSize int,
	callback func(db *gorm.DB, opt interface{}) *gorm.DB,
	opt interface{},
) (*model.Page[*T], error) {
	var objects []*T
	var obj T

	query := r.DB.WithContext(ctx)
	for key, value := range conditions {
		query = query.Where(key, value)
	}
	if callback != nil {
		query = callback(query, opt)
	}

	debug, ok := ctx.Value(utils.DebugKey).(bool)
	if ok && debug {
		query = query.Debug()
	}

	var totalCount int64
	countQuery := query.Model(&obj)
	resultCount := countQuery.Count(&totalCount)
	if resultCount.Error != nil {
		return nil, resultCount.Error
	}

	query = query.Limit(pageSize).Offset((page - 1) * pageSize)
	result := query.Find(&objects)
	if result.Error != nil {
		return nil, result.Error
	}

	return &model.Page[*T]{
		List:      objects,
		PageIndex: page,
		PageSize:  pageSize,
		Total:     totalCount,
	}, nil
}

// GetById 通过 ID 获取记录
func (r *BaseRepository[T]) GetById(ctx context.Context, id uint64, callback func(*gorm.DB) *gorm.DB) (*T, error) {
	var obj T

	query := r.DB.WithContext(ctx).Where("id = ?", id)
	if callback != nil {
		query = callback(query)
	}

	debug, ok := ctx.Value(utils.DebugKey).(bool)
	if ok && debug {
		query = query.Debug()
	}

	result := query.First(&obj)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &obj, nil
}

// GetByUUID 通过 UUID 获取记录
func (r *BaseRepository[T]) GetByUUID(ctx context.Context, uuid string, callback func(*gorm.DB) *gorm.DB) (*T, error) {
	var obj T

	query := r.DB.WithContext(ctx).Where("uuid = ?", uuid)
	if callback != nil {
		query = callback(query)
	}

	debug, ok := ctx.Value(utils.DebugKey).(bool)
	if ok && debug {
		query = query.Debug()
	}

	result := query.First(&obj)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return &obj, nil
}

// GetByCondition 根据条件查询单个记录
func (r *BaseRepository[T]) GetByCondition(ctx context.Context, conditions map[string]interface{}, callback func(*gorm.DB) *gorm.DB) (*T, error) {
	var obj T

	query := r.DB.WithContext(ctx)
	for key, value := range conditions {
		query = query.Where(key, value)
	}
	if callback != nil {
		query = callback(query)
	}

	debug, ok := ctx.Value(utils.DebugKey).(bool)
	if ok && debug {
		query = query.Debug()
	}

	result := query.First(&obj)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}

	return &obj, nil
}

// GetFirst 根据SQL条件查询单个记录
func (r *BaseRepository[T]) GetFirst(ctx context.Context, query interface{}, args ...interface{}) (*T, error) {
	var obj T

	db := r.DB.WithContext(ctx).Where(query, args...)

	debug, ok := ctx.Value(utils.DebugKey).(bool)
	if ok && debug {
		db = db.Debug()
	}

	result := db.First(&obj)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}

	return &obj, nil
}

// Exists 检查记录是否存在
func (r *BaseRepository[T]) Exists(ctx context.Context, query interface{}, args ...interface{}) (bool, error) {
	var count int64

	db := r.DB.WithContext(ctx).Model(new(T)).Where(query, args...)

	debug, ok := ctx.Value(utils.DebugKey).(bool)
	if ok && debug {
		db = db.Debug()
	}

	result := db.Count(&count)
	if result.Error != nil {
		return false, result.Error
	}

	return count > 0, nil
}
