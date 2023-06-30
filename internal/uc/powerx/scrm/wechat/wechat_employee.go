package wechat

import (
    "PowerX/internal/model/powermodel"
    "PowerX/internal/model/scrm/organization"
    "PowerX/internal/types"
    "context"
    "fmt"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/department/response"
    "github.com/zeromicro/go-zero/core/logx"
    "gorm.io/gorm"
    "gorm.io/gorm/clause"
    "strings"
)

//
// SyncDepartmentsAndEmployees
//  @Description:
//  @receiver uc
//  @param ctx
//  @return error
//
func (this *wechatUseCase) SyncDepartmentsAndEmployees(ctx context.Context) error {

    list, err := this.wework.Department.SimpleList(ctx, 1)
    if err != nil {
        logx.Errorf(`scrm.wechat.sync.organization.list.error. %v`, err)
        return err
    }
    this.gLock.Add(len(list.DepartmentIDs))
    for _, val := range list.DepartmentIDs {
        go func(val response.DepartmentID) {
            this.deparment(val)
            this.employee(val)
            this.gLock.Done()
        }(val)

    }
    this.gLock.Wait()
    return err
}

//
// deparment
//  @Description:
//  @receiver this
//  @param val
//
func (this *wechatUseCase) deparment(val response.DepartmentID) {

    detail, err := this.wework.Department.Get(this.ctx, val.ID)
    if err != nil {
        logx.Errorf(`scrm.wechat.sync.organization.info.error. %v`, err)
    }
    dep := organization.WeWorkDepartment{
        WeWorkDepId:      detail.Department.ID,
        Name:             detail.Department.Name,
        NameEn:           detail.Department.NameEN,
        WeWorkParentId:   detail.Department.ParentID,
        Order:            detail.Department.Order,
        DepartmentLeader: strings.Join(detail.Department.DepartmentLeaders, `,`),
    }

    err = this.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "we_work_dep_id"}}, UpdateAll: true}).Create(&dep).Error
    if err != nil {
        logx.Errorf(`scrm.wechat.sync.organization.department.sql.error. %v`, err)
    }

}

//
// employee
//  @Description:
//  @receiver this
//  @param val
//
func (this *wechatUseCase) employee(val response.DepartmentID) {

    users, err := this.wework.User.GetDetailedDepartmentUsers(this.ctx, val.ID, 0)
    if err != nil {
        logx.Errorf(`scrm.wechat.sync.organization.user.error. %v`, err)
    }
    for _, user := range users.UserList {
        open, _ := this.wework.User.UserIdToOpenID(this.ctx, user.UserID)
        emp := organization.WeWorkEmployee{
            WeWorkUserId:           user.UserID,
            Name:                   user.Name,
            Position:               user.Position,
            Mobile:                 user.UserID,
            Email:                  fmt.Sprintf(`%s@to.com`, user.UserID),
            Alias:                  user.Alias,
            OpenUserId:             open.OpenID,
            WeWorkMainDepartmentId: user.MainDepartment,
            Status:                 user.Status,
            QrCode:                 user.QrCode,
            RefEmployeeId:          0,
        }
        err = this.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "we_work_user_id"}}, UpdateAll: true}).Create(&emp).Error
        logx.Errorf(`scrm.wechat.sync.organization.employee.sql.error. %v`, err)
    }

}

//
// buildFindManyEmployeesQueryNoPage
//  @Description:
//  @param query
//  @param opt
//  @return *gorm.DB
//
func buildFindManyEmployeesQueryNoPage(query *gorm.DB, opt *FindManyWechatEmployeesOption) *gorm.DB {
    if len(opt.Ids) > 0 {
        query.Where("id in ?", opt.Ids)
    }
    if len(opt.Names) > 0 {
        query.Where("name in ?", opt.Names)
    }
    if len(opt.Emails) > 0 {
        query.Where("email in ?", opt.Emails)
    }
    if len(opt.Mobile) > 0 {
        query.Where("mobile in ?", opt.Mobile)
    }
    if len(opt.Alias) > 0 {
        query.Where("alias in ?", opt.Alias)
    }
    if len(opt.OpenUserId) > 0 {
        query.Where("open_user_id in ?", opt.OpenUserId)
    }
    if len(opt.WeWorkMainDepartmentId) > 0 {
        query.Where("we_work_main_department_id in ? ", opt.WeWorkMainDepartmentId)
    }
    if len(opt.Status) > 0 {
        query.Where("status in ?", opt.Status)
    }
    return query
}

//
// FindManyWechatEmployeesPage
//  @Description:
//  @receiver uc
//  @param ctx
//  @param opt
//  @return *types.Page[*organization.WeWorkEmployee]
//  @return error
//
func (uc *wechatUseCase) FindManyWechatEmployeesPage(ctx context.Context, opt *types.PageOption[FindManyWechatEmployeesOption]) (*types.Page[*organization.WeWorkEmployee], error) {

    var employees []*organization.WeWorkEmployee
    var count int64
    query := uc.db.WithContext(ctx).Model(&organization.WeWorkEmployee{})

    if opt.PageIndex == 0 {
        opt.PageIndex = 1
    }
    if opt.PageSize == 0 {
        opt.PageSize = powermodel.PageDefaultSize
    }
    if err := query.Count(&count).Error; err != nil {
        return nil, err
    }
    if opt.PageIndex != 0 && opt.PageSize != 0 {
        query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
    }
    query = buildFindManyEmployeesQueryNoPage(query, &opt.Option)

    err := query.Find(&employees).Error

    return &types.Page[*organization.WeWorkEmployee]{
        List:      employees,
        PageIndex: opt.PageIndex,
        PageSize:  opt.PageSize,
        Total:     count,
    }, err
}
