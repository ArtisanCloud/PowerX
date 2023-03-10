package uc

import (
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"context"
	"fmt"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type EmployeeUseCase struct {
	db *gorm.DB
}

func newEmployeeUseCase(db *gorm.DB) *EmployeeUseCase {
	return &EmployeeUseCase{
		db: db,
	}
}

type Gender int8

const (
	GenderUnKnow = iota
	GenderMale
	GenderFemale
)

func (g *Gender) String() string {
	switch *g {
	case GenderMale:
		return "MALE"
	case GenderFemale:
		return "FEMALE"
	default:
		return "UN_KNOW"
	}
}

type EmployeeStatus int8

const (
	EmployeeStatusDisable EmployeeStatus = iota
	EmployeeStatusEnable
)

type Employee struct {
	Account       string `gorm:"unique"`
	Name          string
	NickName      string
	Desc          string
	Position      string
	JobTitle      string
	DepartmentIds pq.Int64Array `gorm:"type:bigint[]"`
	MobilePhone   string
	Gender        *Gender
	Email         string
	ExternalEmail string
	Avatar        string
	Password      string
	Status        *EmployeeStatus
	IsReserved    bool
	IsActivated   bool
	*types.Model
}

func (e *Employee) HashPassword() (err error) {
	if e.Password != "" {
		e.Password, err = hashPassword(e.Password)
	}
	return nil
}

func (e *EmployeeUseCase) VerifyPassword(hashedPwd string, pwd string) bool {
	return verifyPassword(hashedPwd, pwd)
}

func (e *EmployeeUseCase) CreateEmployees(ctx context.Context, employees []*Employee) {
	if len(employees) == 0 {
		return
	}

	// todo handle deleted row

	err := e.db.WithContext(ctx).Model(&Employee{}).CreateInBatches(employees, 50).Error
	if err != nil {
		panic(errors.Wrap(err, "batch insert employees failed"))
	}
	return
}

func (e *EmployeeUseCase) UpdateEmployeeById(ctx context.Context, employee *Employee) {
	err := e.db.WithContext(ctx).Model(&Employee{}).Where(employee.ID).Updates(&employee).Error
	if err != nil {
		panic(errors.Wrap(err, "update employees failed"))
	}
	return
}

func (e *EmployeeUseCase) CountEmployees(ctx context.Context, opt *FindEmployeeOption) int64 {
	var count int64
	query := e.db.WithContext(ctx).Model(&Employee{})
	query = buildFindQueryNoPage(query, opt)
	err := query.Count(&count).Error
	if err != nil {
		panic(errors.Wrap(err, "count employees failed"))
	}
	return count
}

type FindEmployeeOption struct {
	Ids             []int64
	Accounts        []string
	Names           []string
	LikeName        string
	Emails          []string
	LikeEmail       string
	DepIds          []int64
	Positions       []string
	PhoneNumbers    []string
	LikePhoneNumber string
	Statuses        []EmployeeStatus
	PageIndex       int
	PageSize        int
}

func buildFindQueryNoPage(query *gorm.DB, opt *FindEmployeeOption) *gorm.DB {
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
	if len(opt.Positions) > 0 {
		query.Where("position in ?", opt.Positions)
	}
	if len(opt.Accounts) > 0 {
		query.Where("account in ?", opt.Accounts)
	}
	if len(opt.DepIds) > 0 {
		query.Where("? && department_ids", pq.Int64Array(opt.DepIds))
	}
	if len(opt.Statuses) > 0 {
		query.Where("status in ?", opt.Statuses)
	}
	return query
}

func (e *EmployeeUseCase) FindManyEmployees(ctx context.Context, opt *FindEmployeeOption) types.Page[*Employee] {
	var employees []*Employee
	var count int64
	query := e.db.WithContext(ctx).Model(&Employee{})

	if opt.PageIndex != 0 && opt.PageSize != 0 {
		query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}
	query = buildFindQueryNoPage(query, opt)
	if err := query.Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "find employees failed"))
	}
	if err := query.Find(&employees).Error; err != nil {
		panic(errors.Wrap(err, "find employees failed"))
	}
	return types.Page[*Employee]{
		List:      employees,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}
}

func (e *EmployeeUseCase) FindOneEmployee(ctx context.Context, opt *FindEmployeeOption) (*Employee, error) {
	var employee *Employee
	query := e.db.WithContext(ctx).Model(&Employee{})
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}
	query = buildFindQueryNoPage(query, opt)
	if err := query.First(&employee).Error; err != nil {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "未找到该用户")
	}
	return employee, nil
}

func (e *EmployeeUseCase) DeleteEmployee(ctx context.Context, id int64) error {
	result := e.db.WithContext(ctx).Where(Employee{IsReserved: false}, "is_reserved").Delete(&Employee{}, id)
	err := result.Error
	if err != nil {
		panic(errors.Wrap(err, "delete employee failed"))
	}
	if result.RowsAffected == 0 {
		return errorx.WithCause(errorx.ErrBadRequest, "删除失败")
	}
	return nil
}

func (e *EmployeeUseCase) GetAllPositions(ctx context.Context) []string {
	var positions []string
	err := e.db.WithContext(ctx).Model(&Employee{}).Where("position != ''").Distinct("position").Select("position").Pluck("position", &positions).Error
	if err != nil {
		panic(errors.Wrap(err, "pluck employee position failed"))
	}
	return positions
}

const defaultCost = bcrypt.MinCost

// 生成哈希密码
func hashPassword(password string) (hashedPwd string, err error) {
	newPassword, err := bcrypt.GenerateFromPassword([]byte(password), defaultCost)
	if err != nil {
		return "", errors.Wrap(err, "gen pwd failed")
	}
	return string(newPassword), nil
}

// 校验密码
func verifyPassword(hashedPwd string, pwd string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPwd), []byte(pwd))
	return err == nil
}
