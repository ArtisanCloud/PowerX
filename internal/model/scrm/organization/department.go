package organization

import (
    "PowerX/internal/model"
)

type Department struct {
    model.Model
    PDep        *Department   `gorm:"foreignKey:PId"`
    Leader      *Employee     `gorm:"foreignKey:LeaderId"`
    Ancestors   []*Department `gorm:"many2many:department_ancestors;"`
    Name        string        `gorm:"comment:部门名称;column:name" json:"name"`
    PId         int64         `gorm:"comment:部门名ID;column:pid" json:"pid"`
    LeaderId    int64         `gorm:"comment:领导ID;column:leader_id" json:"leader_id"`
    Desc        string        `gorm:"comment:描述;column:desc" json:"desc"`
    PhoneNumber string        `gorm:"comment:部门电话;column:phone_number" json:"phone_number"`
    Email       string        `gorm:"comment:部门邮箱;column:email" json:"email"`
    Remark      string        `gorm:"comment:备注;column:remark" json:"remark"`
    IsReserved  bool          `gorm:"comment:保留;column:is_reserved" json:"is_reserved"`
}

//
// Table
//  @Description:
//  @receiver e
//  @return string
//
func (e Department) TableName() string {
    return `departments`
}
