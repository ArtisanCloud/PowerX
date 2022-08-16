package models

import (
	"fmt"
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	database2 "github.com/ArtisanCloud/PowerX/configs/database"
)

// TableName overrides the table name used by REmployeeToDepartment to `profiles`
func (*REmployeeToDepartment) TableName() string {
	return database2.G_DBConfig.Schemas["default"] + "." + TABLE_NAME_R_EMPLOY_TO_DEPARTMENT
}

type REmployeeToDepartment struct {
	*database.PowerPivot

	LeaderID          string `gorm:"column:leader_id" json:"leaderID"`
	EmployeeReferID   string `gorm:"column:employee_id" json:"employeeID"`
	DepartmentReferID int    `gorm:"column:department_id" json:"departmentID"`
}

const TABLE_NAME_R_EMPLOY_TO_DEPARTMENT = "r_employee_to_department"

const R_EMPLOYEE_TO_DEPARTMNET_FOREIGN_KEY = "employee_id"
const R_EMPLOYEE_TO_DEPARTMNET_JOIN_KEY = "department_id"

func (mdl *REmployeeToDepartment) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_R_EMPLOY_TO_DEPARTMENT
	if needFull {
		tableName = database2.G_DBConfig.Schemas["default"] + "." + tableName
	}
	return tableName
}

func (mdl *REmployeeToDepartment) GetForeignKey() string {
	return R_EMPLOYEE_TO_DEPARTMNET_FOREIGN_KEY
}

func (mdl *REmployeeToDepartment) GetForeignValue() string {
	return mdl.EmployeeReferID
}

func (mdl *REmployeeToDepartment) GetJoinKey() string {
	return R_EMPLOYEE_TO_DEPARTMNET_JOIN_KEY
}

func (mdl *REmployeeToDepartment) GetJoinValue() string {
	return fmt.Sprintf("%d", mdl.DepartmentReferID)
}

func (mdl *REmployeeToDepartment) MakePivotsFromEmployeeAndDepartmentIDs(employee *Employee, departmentIDs []int) ([]database.PivotInterface, error) {
	pivots := []database.PivotInterface{}
	for _, departmentID := range departmentIDs {
		pivot := &REmployeeToDepartment{
			EmployeeReferID:   employee.WXUserID.String,
			DepartmentReferID: departmentID,
		}
		pivots = append(pivots, pivot)
	}
	return pivots, nil
}

func (mdl *REmployeeToDepartment) MakePivotsFromEmployeeAndDepartments(employee *Employee, departments []*wx.WXDepartment) ([]database.PivotInterface, error) {
	pivots := []database.PivotInterface{}
	for _, department := range departments {
		pivot := &REmployeeToDepartment{
			EmployeeReferID:   employee.WXUserID.String,
			DepartmentReferID: department.ID,
		}
		pivots = append(pivots, pivot)
	}
	return pivots, nil
}
