package models

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/config"
)

// TableName overrides the table name used by REmployeeToDepartment to `profiles`
func (*REmployeeToDepartment) TableName() string {
	return config.DatabaseConn.Schemas["default"] + "." + TABLE_NAME_R_EMPLOY_TO_DEPARTMENT
}

type REmployeeToDepartment struct {
	*database.PowerPivot

	LeaderID     string `gorm:"column:leader_id" json:"leaderID"`
	EmployeeID   int    `gorm:"column:employee_id" json:"employeeID"`
	DepartmentID int    `gorm:"column:department_id" json:"departmentID"`
}

const TABLE_NAME_R_EMPLOY_TO_DEPARTMENT = "r_employee_to_department"

func (mdl *REmployeeToDepartment) GetTableName(needFull bool) string {
	tableName := wx.TABLE_NAME_DEPARTMENT
	if needFull {
		tableName = config.DatabaseConn.Schemas["default"] + "." + tableName
	}
	return tableName
}
