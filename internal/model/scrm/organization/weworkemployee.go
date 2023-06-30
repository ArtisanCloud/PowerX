package organization

import (
    "PowerX/internal/model"
)

type WeWorkEmployee struct {
    model.Model

    WeWorkUserId           string `gorm:"unique"`
    Name                   string
    Position               string
    Mobile                 string //`gorm:"unique"`
    Gender                 string
    Email                  string //`gorm:"unique"`
    BizMail                string
    Avatar                 string
    ThumbAvatar            string
    Telephone              string
    Alias                  string
    Address                string
    OpenUserId             string
    WeWorkMainDepartmentId int
    Status                 int
    QrCode                 string

    RefEmployeeId int64
}

//
// Table
//  @Description:
//  @receiver e
//  @return string
//
func (e WeWorkEmployee) Table() string {
    return `we_work_employees`
}
