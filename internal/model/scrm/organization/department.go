package organization

import (
    "PowerX/internal/model"
)

type Department struct {
    model.Model
    Name        string
    PId         int64
    PDep        *Department `gorm:"foreignKey:PId"`
    LeaderId    int64
    Leader      *Employee     `gorm:"foreignKey:LeaderId"`
    Ancestors   []*Department `gorm:"many2many:department_ancestors;"`
    Desc        string
    PhoneNumber string
    Email       string
    Remark      string
    IsReserved  bool
}

//
// Table
//  @Description:
//  @receiver e
//  @return string
//
func (e Department) Table() string {
    return `departments`
}
