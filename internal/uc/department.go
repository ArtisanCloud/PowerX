package uc

import (
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type DepartmentUseCase struct {
	db *gorm.DB
}

func newDepartmentUseCase(db *gorm.DB) *DepartmentUseCase {
	return &DepartmentUseCase{
		db: db,
	}
}

type Department struct {
	Name        string
	PId         int64
	AncestorIds pq.Int64Array `gorm:"type:bigint[]"`
	LeaderIds   pq.Int64Array `gorm:"type:bigint[]"`
	Desc        string
	PhoneNumber string
	Email       string
	Remark      string
	IsReserved  bool
	*types.Model
}

func defaultDepartment() *Department {
	return &Department{
		Name:       "组织架构",
		PId:        0,
		Desc:       "根节点, 别删除",
		IsReserved: true,
	}
}

func (d *DepartmentUseCase) Init() {
	var count int64
	if err := d.db.Model(&Department{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init root dep failed"))
	}
	if count == 0 {
		dep := defaultDepartment()
		if err := d.db.Model(&Department{}).Create(&dep).Error; err != nil {
			panic(errors.Wrap(err, "init root dep failed"))
		}
	}
}

func (d *DepartmentUseCase) CreateDepartment(ctx context.Context, dep *Department) error {
	if dep.PId == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "必须指定父部门Id")
	}
	// 查询父节点
	var pDep *Department
	if err := d.db.WithContext(ctx).Model(&Department{}).Where(dep.PId).First(&pDep).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errorx.WithCause(errorx.ErrBadRequest, "父部门不存在")
		}
		panic(errors.Wrap(err, "query parent Dep failed"))
	}
	dep.AncestorIds = append(pDep.AncestorIds, pDep.ID)
	if err := d.db.WithContext(ctx).Model(&Department{}).Create(dep).Error; err != nil {
		panic(errors.Wrap(err, "create dep failed"))
	}
	return nil
}

func (d *DepartmentUseCase) CountDepartments(ctx context.Context) int64 {
	var count int64
	if err := d.db.WithContext(ctx).Model(&Department{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "count departments failed"))
	}
	return count
}

type FindManyDepartmentsOption struct {
	RootId    int64
	DepIds    []int64
	PageIndex int
	PageSize  int
}

func (d *DepartmentUseCase) FindManyDepartments(ctx context.Context, option *FindManyDepartmentsOption) *types.Page[*Department] {
	var deps []*Department
	var count int64
	query := d.db.WithContext(ctx).Model(&Department{})
	// case: set RootId, 将返回自身及其下所有部门
	if option.RootId > 0 {
		query.Where("id = ? or ? = ANY (ancestor_ids)", option.RootId, option.RootId)
		if err := query.Find(&deps).Error; err != nil {
			panic(errors.Wrap(err, "query deps failed"))
		}
		l := len(deps)
		return &types.Page[*Department]{
			List:      deps,
			PageIndex: 1,
			PageSize:  l,
			Total:     int64(l),
		}
	}
	// else
	if err := query.Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "query deps failed"))
	}

	if len(option.DepIds) > 0 {
		query.Where(option.DepIds)
	}

	if option.PageIndex != 0 && option.PageSize != 0 {
		query.Offset((option.PageIndex - 1) * option.PageSize).Limit(option.PageSize)
	}
	if err := query.Find(&deps).Error; err != nil {
		panic(errors.Wrap(err, "query deps failed"))
	}
	return &types.Page[*Department]{
		List:      deps,
		PageIndex: option.PageIndex,
		PageSize:  option.PageSize,
		Total:     count,
	}
}

type FindOneDepartmentOption struct {
	Id      *int64
	DepName string
}

func (d *DepartmentUseCase) FindOneDepartment(ctx context.Context, option *FindOneDepartmentOption) (*Department, error) {
	var dep Department
	query := d.db.WithContext(ctx).Model(&Department{})
	if option.Id != nil {
		query.Where(*option.Id)
	}
	if option.DepName != "" {
		query.Where(option.DepName)
	}
	if err := query.First(&dep).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("未查找到部门")
		}
		panic(errors.Wrap(err, "find dep failed"))
	}
	return &dep, nil
}

func (d *DepartmentUseCase) DeleteDepartmentById(ctx context.Context, id int64) error {
	err := d.db.Transaction(func(tx *gorm.DB) error {
		result := tx.WithContext(ctx).Delete(&Department{}, id).Where(Department{IsReserved: false})
		if err := result.Error; err != nil {
			return err
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return tx.WithContext(ctx).Where("? = ANY (ancestor_ids)", id).Delete(&Department{}).Error
	})
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return errorx.WithCause(errorx.ErrBadRequest, "未查找到要删除的部门")
		}
		panic(errors.Wrap(err, "delete department failed"))
	}
	return nil
}
