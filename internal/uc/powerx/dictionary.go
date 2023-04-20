package powerx

import (
	"PowerX/internal/model"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

type DataDictionaryUseCase struct {
	db *gorm.DB
}

func NewDataDictionaryUseCase(db *gorm.DB) *DataDictionaryUseCase {
	return &DataDictionaryUseCase{
		db: db,
	}
}
func (uc *DataDictionaryUseCase) CreateDataDictionaryItem(ctx context.Context, dd *model.DataDictionaryItem) error {
	if err := uc.db.WithContext(ctx).Create(&dd).Error; err != nil {
		// todo use errors.Is() when gorm update ErrDuplicatedKey
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrBadRequest, "数据字典项已存在")
		}
		panic(err)
	}
	return nil
}

func (uc *DataDictionaryUseCase) PatchDataDictionaryItem(ctx context.Context, dictType string, key string, dd *model.DataDictionaryItem) error {
	result := uc.db.WithContext(ctx).Model(&dd).
		Clauses(clause.Returning{}).
		Where("key = ? AND type = ?", key, dictType).
		Updates(&dd)
	if result.Error != nil {
		panic(result.Error)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到数据字典")
	}
	return nil
}

func (uc *DataDictionaryUseCase) GetDataDictionaryItem(ctx context.Context, dictType string, key string) (*model.DataDictionaryItem, error) {
	var dd model.DataDictionaryItem
	if err := uc.db.WithContext(ctx).Where("key = ? AND type = ?", key, dictType).First(&dd).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到数据字典")
		}
		panic(err)
	}
	return &dd, nil
}

type FindManyDataDictItemOption struct {
	Types        []string
	Keys         []string
	LikeItemName string
	types.PageEmbedOption
}

func (uc *DataDictionaryUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyDataDictItemOption) *gorm.DB {
	if len(opt.Types) > 0 {
		db = db.Where("type IN ?", opt.Types)
	}
	if len(opt.Keys) > 0 {
		db = db.Where("key IN ?", opt.Keys)
	}
	if opt.LikeItemName != "" {
		db = db.Where("name LIKE ?", "%"+opt.LikeItemName+"%")
	}

	return db
}

func (uc *DataDictionaryUseCase) FindAllDictionaryItems(ctx context.Context, opt *FindManyDataDictItemOption) (dictionaryItems []*model.DataDictionaryItem, err error) {
	var count int64
	query := uc.db.WithContext(ctx).Model(&model.DataDictionaryItem{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err = query.Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "find all dictionaryItems failed"))
	}
	if err := query.
		//Debug().
		Find(&dictionaryItems).Error; err != nil {
		panic(errors.Wrap(err, "find all dictionaryItems failed"))
	}
	return dictionaryItems, err
}

func (uc *DataDictionaryUseCase) CountDataDictionaryItems(ctx context.Context, opt *FindManyDataDictItemOption) int64 {
	db := uc.db.WithContext(ctx).Model(&model.DataDictionaryItem{})

	db = uc.buildFindQueryNoPage(db, opt)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		panic(err)
	}
	return count
}

func (uc *DataDictionaryUseCase) FindManyDataDictionaryItem(ctx context.Context, opt *FindManyDataDictItemOption) (types.Page[*model.DataDictionaryItem], error) {
	opt.DefaultPageIfNotSet()
	var dds []*model.DataDictionaryItem
	db := uc.db.WithContext(ctx).Model(&model.DataDictionaryItem{})

	db = uc.buildFindQueryNoPage(db, opt)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		panic(err)
	}

	if err := db.Order("type").Order("key").Order("sort").Find(&dds).Error; err != nil {
		panic(err)
	}

	return types.Page[*model.DataDictionaryItem]{
		List:      dds,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *DataDictionaryUseCase) DeleteDataDictionaryItem(ctx context.Context, dictType string, key string) error {
	if !uc.ItemIsExist(ctx, dictType, key) {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到数据字典数据项")
	}

	result := uc.db.WithContext(ctx).Unscoped().Where("key = ? AND type = ?", key, dictType).Delete(&model.DataDictionaryItem{})
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未删除该制定数据字典数据项")
	}
	return nil
}

func (uc *DataDictionaryUseCase) TypeIsExist(ctx context.Context, dictType string) bool {
	var count int64
	err := uc.db.WithContext(ctx).Model(&model.DataDictionaryType{}).Where("type = ?", dictType).Count(&count).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true
		}
		panic(err)
	}

	return count > 0
}

func (uc *DataDictionaryUseCase) CreateDataDictionaryType(ctx context.Context, ddt *model.DataDictionaryType) error {
	if err := uc.db.WithContext(ctx).Create(&ddt).Error; err != nil {
		// todo use errors.Is() when gorm update ErrDuplicatedKey
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrBadRequest, "数据字典类型已存在")
		}
		panic(err)
	}
	return nil
}

func (uc *DataDictionaryUseCase) PatchDataDictionaryType(ctx context.Context, dictType string, ddt *model.DataDictionaryType) error {
	result := uc.db.WithContext(ctx).Model(&ddt).
		Clauses(clause.Returning{}).
		Where("type = ?", dictType).
		Updates(&ddt)
	if result.Error != nil {
		panic(result.Error)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到数据字典类型")
	}
	return nil
}

func (uc *DataDictionaryUseCase) GetDataDictionaryType(ctx context.Context, dictType string) (*model.DataDictionaryType, error) {
	var ddt model.DataDictionaryType
	if err := uc.db.WithContext(ctx).Where("type = ?", dictType).
		Preload("Items").
		First(&ddt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到数据字典类型")
		}
		panic(err)
	}
	return &ddt, nil
}

type FindManyDataDictTypeOption struct {
	Types        []string
	LikeTypeName string
	types.PageEmbedOption
}

func (uc *DataDictionaryUseCase) FindManyDataDictionaryType(ctx context.Context, opt *FindManyDataDictTypeOption) (types.Page[*model.DataDictionaryType], error) {
	opt.DefaultPageIfNotSet()
	var ddts []*model.DataDictionaryType
	db := uc.db.WithContext(ctx).Model(&model.DataDictionaryType{})
	if len(opt.Types) > 0 {
		db = db.Where("type IN ?", opt.Types)
	}
	if opt.LikeTypeName != "" {
		db = db.Where("name LIKE ?", "%"+opt.LikeTypeName+"%")
	}

	opt.DefaultPageIfNotSet()
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		db.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	var count int64
	if err := db.Count(&count).Error; err != nil {
		panic(err)
	}

	if err := db.Order("type").
		Preload("Items").
		Debug().
		Find(&ddts).Error; err != nil {
		panic(err)
	}

	return types.Page[*model.DataDictionaryType]{
		List:      ddts,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *DataDictionaryUseCase) DeleteDataDictionaryType(ctx context.Context, dictType string) error {
	if uc.CountDataDictionaryItems(ctx, &FindManyDataDictItemOption{
		Types: []string{dictType},
	}) > 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "数据字典类型下存在数据字典，无法删除")
	}

	result := uc.db.WithContext(ctx).Unscoped().Where("type = ?", dictType).Delete(&model.DataDictionaryType{})
	if err := result.Error; err != nil {
		panic(err)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到数据字典类型")
	}
	return nil
}

func (uc *DataDictionaryUseCase) ItemIsExist(ctx context.Context, dictType string, key string) bool {
	var count int64
	err := uc.db.WithContext(ctx).Model(&model.DataDictionaryItem{}).
		Where("type = ? AND key = ?", dictType, key).
		Count(&count).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true
		}
		panic(err)
	}

	return count > 0
}
