package powerx

import (
	"PowerX/internal/model"
	"PowerX/internal/model/option"
	"PowerX/internal/model/origanzation"
	"PowerX/internal/model/permission"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"PowerX/pkg/slicex"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

type OrganizationUseCase struct {
	db *gorm.DB
}

func NewOrganizationUseCase(db *gorm.DB) *OrganizationUseCase {
	return &OrganizationUseCase{
		db: db,
	}
}

func (uc *OrganizationUseCase) VerifyPassword(hashedPwd string, pwd string) bool {
	return origanzation.VerifyPassword(hashedPwd, pwd)
}

func (uc *OrganizationUseCase) CreateEmployee(ctx context.Context, employee *origanzation.Employee) (err error) {
	if err := uc.db.WithContext(ctx).Create(&employee).Error; err != nil {
		// todo use errors.Is() when gorm update ErrDuplicatedKey
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return errorx.WithCause(errorx.ErrBadRequest, "账号已存在")
		}
		panic(err)
	}
	return nil
}

func (uc *OrganizationUseCase) FindAccountsByIds(ctx context.Context, employeeIds []int64) (accounts []string) {
	err := uc.db.WithContext(ctx).Model(&origanzation.Employee{}).Where("id in ?", employeeIds).Pluck("account", &accounts).Error
	if err != nil {
		panic(errors.Wrap(err, "find accounts by ids failed"))
	}
	return accounts
}

func (uc *OrganizationUseCase) PatchEmployeeByUserId(ctx context.Context, employee *origanzation.Employee, employeeId int64) error {
	result := uc.db.WithContext(ctx).Model(&origanzation.Employee{}).Where(employee.Id).Updates(&employee)
	if result.Error != nil {
		panic(result.Error)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "未找到员工")
	}
	return nil
}

func buildFindManyEmployeesQueryNoPage(query *gorm.DB, opt *option.FindManyEmployeesOption) *gorm.DB {
	if len(opt.Ids) > 0 {
		query.Where("id in ?", opt.Ids)
	}
	if len(opt.Names) > 0 {
		query.Where("name in ?", opt.Names)
	} else if opt.LikeName != "" {
		query.Where("name like ?", fmt.Sprintf("%s%%", opt.LikeName))
	}
	if len(opt.Emails) > 0 {
		query.Where("email in ?", opt.Emails)
	} else if opt.LikeEmail != "" {
		query.Where("email like ?", fmt.Sprintf("%s%%", opt.LikeEmail))
	}
	if len(opt.PhoneNumbers) > 0 {
		query.Where("mobile_phone in ?")
	} else if opt.LikePhoneNumber != "" {
		query.Where("mobile_phone like ?", fmt.Sprintf("%s%%", opt.LikePhoneNumber))
	}
	if len(opt.PositionIDs) > 0 {
		query.Where("position_id in ?", opt.PositionIDs)
	}
	if len(opt.Accounts) > 0 {
		query.Where("account in ?", opt.Accounts)
	}
	if len(opt.DepIds) > 0 {
		query.Where("department_id in (?)", opt.DepIds)
	}
	if len(opt.Statuses) > 0 {
		query.Where("status in ?", opt.Statuses)
	}
	return query
}

func (uc *OrganizationUseCase) FindManyEmployeesPage(ctx context.Context, opt *option.FindManyEmployeesOption) types.Page[*origanzation.Employee] {
	var employees []*origanzation.Employee
	var count int64
	query := uc.db.WithContext(ctx).Model(&origanzation.Employee{})

	query = buildFindManyEmployeesQueryNoPage(query, opt)
	if err := query.Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "find employees failed"))
	}

	if opt.PageIndex == 0 {
		opt.PageIndex = 1
	}
	if opt.PageSize == 0 {
		opt.PageSize = powermodel.PageDefaultSize
	}

	if opt.PageIndex != 0 && opt.PageSize != 0 {
		query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	if err := query.Find(&employees).Error; err != nil {
		panic(errors.Wrap(err, "find employees failed"))
	}
	return types.Page[*origanzation.Employee]{
		List:      employees,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}
}

func (uc *OrganizationUseCase) FindOneEmployeeByLoginOption(ctx context.Context, opt *option.EmployeeLoginOption) (employee *origanzation.Employee, err error) {
	if *opt == (option.EmployeeLoginOption{}) {
		panic(errors.New("option empty"))
	}

	var queryEmployee origanzation.Employee
	if opt.Account != "" {
		queryEmployee.Account = opt.Account
	}
	if opt.Email != "" {
		queryEmployee.Email = opt.Email
	}
	if opt.PhoneNumber != "" {
		queryEmployee.MobilePhone = opt.PhoneNumber
	}

	if err = uc.db.WithContext(ctx).Model(&origanzation.Employee{}).Where(&queryEmployee).First(&employee).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "用户不存在, 请检查登录信息")
		}
		panic(err)
	}
	return
}

func (uc *OrganizationUseCase) FindOneEmployeeById(ctx context.Context, id int64) (employee *origanzation.Employee, err error) {
	if err = uc.db.WithContext(ctx).Where(id).Preload("Department").Preload("Position").First(&employee).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "用户不存在")
		}
		panic(err)
	}
	return
}

func (uc *OrganizationUseCase) UpdateEmployeeById(ctx context.Context, employee *origanzation.Employee, employeeId int64) error {
	whereCase := origanzation.Employee{
		Model: model.Model{
			Id: employeeId,
		},
		IsReserved: false,
	}
	result := uc.db.WithContext(ctx).Where(whereCase, "is_reserved").Clauses(&clause.Returning{}).Updates(employee)
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "更新失败, 用户保留或不存在")
	}
	err := result.Error
	if err != nil {
		panic(errors.Wrap(err, "delete employee failed"))
	}
	return nil
}

func (uc *OrganizationUseCase) DeleteEmployeeById(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Where(origanzation.Employee{IsReserved: false}, "is_reserved").Delete(&origanzation.Employee{}, id)
	err := result.Error
	if err != nil {
		panic(errors.Wrap(err, "delete employee failed"))
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "删除失败")
	}
	return nil
}

// FindEmployeePositionRoleCodes 获取员工的职位的角色代码
func (uc *OrganizationUseCase) FindEmployeePositionRoleCodes(ctx context.Context, employeeId int64) (roleCodes []string, err error) {
	var employee origanzation.Employee
	if err = uc.db.WithContext(ctx).Preload("Position.Roles").First(&employee, employeeId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "用户不存在")
		}
		panic(err)
	}
	roleCodes = slicex.SlicePluck(employee.Position.Roles, func(item *permission.AdminRole) string {
		return item.RoleCode
	})
	return
}

func (uc *OrganizationUseCase) FindOneDepartment(ctx context.Context, id int64) (department *origanzation.Department, err error) {
	department = &origanzation.Department{}
	if err := uc.db.WithContext(ctx).Preload("Leader").First(department, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "部门未找到")
		}
		panic(err)
	}
	return department, nil
}

func (uc *OrganizationUseCase) CreateDepartment(ctx context.Context, dep *origanzation.Department) error {
	if dep.PId == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "必须指定父部门Id")
	}
	db := uc.db.WithContext(ctx)
	// 查询父节点
	var pDep *origanzation.Department
	if err := db.Preload("Ancestors").Where(dep.PId).First(&pDep).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errorx.WithCause(errorx.ErrBadRequest, "父部门不存在")
		}
		panic(errors.Wrap(err, "query parent Dep failed"))
	}
	for _, ancestor := range pDep.Ancestors {
		dep.Ancestors = append(dep.Ancestors, ancestor)
	}
	dep.Ancestors = append(dep.Ancestors, pDep)

	if err := db.Create(dep).Error; err != nil {
		panic(errors.Wrap(err, "create dep failed"))
	}
	return nil
}

func (uc *OrganizationUseCase) FindManyDepartmentsPage(ctx context.Context, opt *types.PageOption[option.FindManyDepartmentsOption]) *types.Page[*origanzation.Department] {
	var deps []*origanzation.Department
	var count int64
	query := uc.db.WithContext(ctx).Model(origanzation.Department{})

	if len(opt.Option.DepIds) > 0 {
		query.Where(opt.Option.DepIds)
	}

	if opt.Option.LikeName != "" {
		query.Where("name like ?", "%"+opt.Option.LikeName+"%")
	}

	if err := query.Count(&count).Error; err != nil {
		panic(err)
	}
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}
	if err := query.Find(&deps).Error; err != nil {
		panic(errors.Wrap(err, "query deps failed"))
	}
	return &types.Page[*origanzation.Department]{
		List:      deps,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}
}

func (uc *OrganizationUseCase) FindManyDepartmentsByRootId(ctx context.Context, rootId int64) (departments []*origanzation.Department, err error) {
	departments = []*origanzation.Department{}
	if err := uc.db.WithContext(ctx).Model(origanzation.Department{}).Preload("Leader").Preload("Ancestors").
		Joins("LEFT JOIN department_ancestors ON departments.id = department_ancestors.department_id").
		Where("department_ancestors.ancestor_id = ?", rootId).Or("departments.id = ?", rootId).
		Find(&departments).Error; err != nil {
		panic(err)
	}
	if len(departments) == 0 {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "根部门不存在")
	}
	return
}

func (uc *OrganizationUseCase) FindAllDepartments(ctx context.Context) (departments []*origanzation.Department) {
	if err := uc.db.WithContext(ctx).Preload("Leader").Find(&departments).Error; err != nil {
		panic(err)
	}
	return
}

func (uc *OrganizationUseCase) CountEmployeeInDepartmentByIds(ctx context.Context, depIds []int64) (count int64) {
	if err := uc.db.WithContext(ctx).Model(origanzation.Employee{}).Where("department_id in ?", depIds).Count(&count).Error; err != nil {
		panic(err)
	}
	return count
}

func (uc *OrganizationUseCase) PatchDepartmentById(ctx context.Context, id int64, dep *origanzation.Department) error {
	result := uc.db.WithContext(ctx).Model(origanzation.Department{}).Where(origanzation.Department{IsReserved: false}, "is_reserved").
		Where("id = ?", id).Updates(&dep)
	if result.Error != nil {
		panic(result.Error)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "更新失败")
	}
	return nil
}

func (uc *OrganizationUseCase) DeleteDepartmentById(ctx context.Context, id int64) error {
	db := uc.db.WithContext(ctx)
	queryWhere := db.Model(origanzation.Department{}).
		Joins("LEFT JOIN department_ancestors ON departments.id = department_ancestors.department_id").
		Where("department_ancestors.ancestor_id = ?", id).Or("departments.id = ?", id).
		Select("id")
	result := db.Model(origanzation.Department{}).Where(origanzation.Department{IsReserved: false}, "is_reserved").
		Where("id in (?)", queryWhere).Delete(&origanzation.Department{})
	if result.Error != nil {
		panic(result.Error)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "删除失败")
	}
	return nil
}

// CreatePosition 创建职位
func (uc *OrganizationUseCase) CreatePosition(ctx context.Context, position *origanzation.Position) error {
	if err := uc.db.WithContext(ctx).Create(&position).Error; err != nil {
		panic(err)
	}
	return nil
}

// EditPosition 编辑职位
func (uc *OrganizationUseCase) EditPosition(ctx context.Context, position *origanzation.Position) error {
	result := uc.db.WithContext(ctx).Model(&origanzation.Position{}).Where("id = ?", position.Id).Updates(&position)
	if result.Error != nil {
		panic(result.Error)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "更新失败")
	}
	return nil
}

// FindOnePositionByID 查询职位列表
func (uc *OrganizationUseCase) FindOnePositionByID(ctx context.Context, id int64) (position *origanzation.Position, err error) {
	position = &origanzation.Position{}
	if err := uc.db.WithContext(ctx).Preload("Roles").First(position, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errorx.WithCause(errorx.ErrBadRequest, "职位未找到")
		}
		panic(err)
	}
	return position, nil
}

// FindManyPositions 查询职位列表
func (uc *OrganizationUseCase) FindManyPositions(ctx context.Context, opt *option.FindManyPositionsOption) (positions []*origanzation.Position, err error) {
	positions = []*origanzation.Position{}
	query := uc.db.WithContext(ctx).Model(&origanzation.Position{}).Preload("Roles")

	if opt.LikeName != "" {
		query = query.Where("name like ?", fmt.Sprintf("%%%s%%", opt.LikeName))
	}

	if err := query.Find(&positions).Error; err != nil {
		panic(errors.Wrap(err, "query positions failed"))
	}
	return positions, nil
}

// DeletePosition 删除职位
func (uc *OrganizationUseCase) DeletePosition(ctx context.Context, id int64) error {
	result := uc.db.WithContext(ctx).Where("id = ?", id).Delete(&origanzation.Position{})
	if result.Error != nil {
		panic(result.Error)
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "删除失败")
	}
	return nil
}

// PatchPosition 编辑职位
func (uc *OrganizationUseCase) PatchPosition(ctx context.Context, id int64, position *origanzation.Position) error {
	position.Id = id
	result := uc.db.WithContext(ctx).Where("id = ?", id).Updates(&position)
	uc.db.Model(&position).Association("Roles").Replace(position.Roles)
	if result.Error != nil {
		panic(result.Error)
	}
	return nil
}

// GetPositionOptionMap 获取职位Option {label: Name, value: Id}
func (uc *OrganizationUseCase) GetPositionOptionMap(ctx context.Context, search string) ([]map[string]any, error) {
	var positions []*origanzation.Position
	query := uc.db.WithContext(ctx).Model(&origanzation.Position{})
	if search != "" {
		query = query.Where("name like ?", fmt.Sprintf("%%%s%%", search))
	}
	if err := query.Find(&positions).Error; err != nil {
		panic(err)
	}
	var optionMapList []map[string]any
	for _, position := range positions {
		om := map[string]any{
			"label": position.Name,
			"value": position.Id,
		}
		optionMapList = append(optionMapList, om)
	}
	return optionMapList, nil
}
