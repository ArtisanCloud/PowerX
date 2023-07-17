package app

import (
    "PowerX/internal/model"
    "gorm.io/gorm"
    "gorm.io/gorm/clause"
)

type WeWorkAppGroup struct {
    model.Model

    Name     string `gorm:"comment:群名称;column:name" json:"name"`
    Owner    string `gorm:"comment:群主;column:owner" json:"owner"`
    UserList string `gorm:"comment:群用户;column:user_list" json:"user_list"`
    ChatID   string `gorm:"comment:群ID;unique"`
}

//
// Table
//  @Description:
//  @receiver e
//  @return string
//
func (e WeWorkAppGroup) TableName() string {
    return `we_work_app_groups`
}

type (
    AdapterGroupSliceChatIDs func(groups []*WeWorkAppGroup) (ids []string)
)

//
// Query
//  @Description:
//  @receiver this
//  @param db
//  @return groups
//  @return err
//
func (e *WeWorkAppGroup) Query(db *gorm.DB) (groups []*WeWorkAppGroup) {

    err := db.Model(e).Find(&groups).Error
    if err != nil {
        panic(err)
    }
    return groups

}

//
// Action
//  @Description:
//  @receiver this
//  @param db
//  @param group
//  @return []*WeWorkAppGroup
//
func (e *WeWorkAppGroup) Action(db *gorm.DB, group []*WeWorkAppGroup) {

    err := db.Table(e.TableName()).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "chat_id"}}, UpdateAll: true}).Create(&group).Error
    if err != nil {
        panic(err)
    }

}
