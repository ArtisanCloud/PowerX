package models

import (
	"fmt"
	databasePowerLib "github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	databaseConfig "github.com/ArtisanCloud/PowerX/config"
)

// TableName overrides the table name used by REmployeeToDepartment to `profiles`
func (mdl *REmployeeToDepartment) TableName() string {
	return mdl.GetTableName(true)
}

type REmployeeToDepartment struct {
	*databasePowerLib.PowerPivot

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
		tableName = databasePowerLib.GetTableFullName(databaseConfig.G_DBConfig.Schemas.Default, databaseConfig.G_DBConfig.Prefix, tableName)
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

func (mdl *REmployeeToDepartment) MakePivotsFromEmployeeAndDepartmentIDs(employee *Employee, departmentIDs []int) ([]databasePowerLib.PivotInterface, error) {
	pivots := []databasePowerLib.PivotInterface{}
	for _, departmentID := range departmentIDs {
		pivot := &REmployeeToDepartment{
			EmployeeReferID:   employee.WXUserID.String,
			DepartmentReferID: departmentID,
		}
		pivots = append(pivots, pivot)
	}
	return pivots, nil
}

func (mdl *REmployeeToDepartment) MakePivotsFromEmployeeAndDepartments(employee *Employee, departments []*wx.WXDepartment) ([]databasePowerLib.PivotInterface, error) {
	pivots := []databasePowerLib.PivotInterface{}
	for _, department := range departments {
		pivot := &REmployeeToDepartment{
			EmployeeReferID:   employee.WXUserID.String,
			DepartmentReferID: department.ID,
		}
		pivots = append(pivots, pivot)
	}
	return pivots, nil
}
