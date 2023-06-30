package organization

import (
    "PowerX/internal/model"
)

type WeWorkDepartment struct {
    model.Model
    // Leader         *WeWorkEmployee `gorm:"foreignKey:LeaderId"`
    WeWorkDepId      int `gorm:"unique"`
    Name             string
    NameEn           string
    WeWorkParentId   int
    Order            int
    DepartmentLeader string
    RefDepartmentId  int64
}

//
// Table
//  @Description:
//  @receiver e
//  @return string
//
func (e WeWorkDepartment) Table() string {
    return `we_work_departments`
}
