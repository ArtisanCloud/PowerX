package organization

import (
    "PowerX/internal/model"
    "gorm.io/gorm"
    "gorm.io/gorm/clause"
)

type WeWorkEmployee struct {
    model.Model

    WeWorkUserId           string `gorm:"comment:员工ID;column:we_work_user_id;unique" json:"we_work_user_id"`
    Name                   string `gorm:"comment:员工名称;column:name" json:"name"`
    Position               string `gorm:"comment:员工位置;column:position" json:"position"`
    Mobile                 string `gorm:"comment:员工电话;column:mobile" json:"mobile"`
    Gender                 string `gorm:"comment:员工性别;column:gender" json:"gender"`
    Email                  string `gorm:"comment:邮箱;column:email" json:"email"`
    BizMail                string `gorm:"comment:商务邮箱;column:biz_mail" json:"biz_mail"`
    Avatar                 string `gorm:"comment:头像;column:avatar" json:"avatar"`
    ThumbAvatar            string `gorm:"comment:ThumbAvatar;column:thumb_avatar" json:"thumb_avatar"`
    Telephone              string `gorm:"comment:电话;column:telephone" json:"telephone"`
    Alias                  string `gorm:"comment:别称;column:alias" json:"alias"`
    Address                string `gorm:"comment:地址;column:address" json:"address"`
    OpenUserId             string `gorm:"comment:开放ID;column:open_user_id" json:"open_user_id"`
    WeWorkMainDepartmentId int    `gorm:"comment:部门ID;column:we_work_main_department_id" json:"we_work_main_department_id"`
    Status                 int    `gorm:"comment:状态;column:status" json:"status"`
    QrCode                 string `gorm:"comment:二维码;column:qr_code" json:"qr_code"`
    Department             string `gorm:"comment:部门;column:department" json:"department"`
    RefEmployeeId          int64  `gorm:"comment:RefEmployeeId;column:ref_employee_id" json:"ref_employee_id"`
}

//
// Table
//  @Description:
//  @receiver e
//  @return string
//
func (e WeWorkEmployee) TableName() string {
    return `we_work_employees`
}

type (
    AdapterEmployeeSliceUserIDs func(employee []*WeWorkEmployee) (ids []string)
)

//
// Query
//  @Description:
//  @receiver this
//  @param db
//  @return employees
//
func (e WeWorkEmployee) Query(db *gorm.DB) (employees []*WeWorkEmployee) {

    err := db.Model(e).Find(&employees).Error
    if err != nil {
        panic(err)
    }
    return employees

}

//
// Action
//  @Description:
//  @receiver e
//  @param db
//  @param employees
//
func (e WeWorkEmployee) Action(db *gorm.DB, employees []*WeWorkEmployee) {

    err := db.Table(e.TableName()).Debug().Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "we_work_user_id"}}, UpdateAll: true}).CreateInBatches(&employees, 100).Error
    if err != nil {
        panic(err)
    }

}
