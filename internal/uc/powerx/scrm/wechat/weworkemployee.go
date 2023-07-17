package wechat

import (
    "PowerX/internal/model/powermodel"
    "PowerX/internal/model/scrm/organization"
    "PowerX/internal/types"
    "context"
    "fmt"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/department/response"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/user/request"
    "gorm.io/gorm"
    "strings"
)

//
// CreateWeWorkEmployeeRequest
//  @Description:
//  @receiver uc
//  @param ctx
//  @param dep
//  @return error
//
func (this *wechatUseCase) CreateWeWorkEmployeeRequest(ctx context.Context, employee *organization.WeWorkEmployee) (err error) {

    create, err := this.wework.User.Create(ctx, this.employeeModelToWeWorkRequest(employee))
    if err != nil {
        panic(err)
    } else {
        err = this.help.error(`scrm.create.wework.employee.error`, *create)
    }

    if err == nil {
        this.modelWeworkOrganization.employee.Action(this.db, []*organization.WeWorkEmployee{employee})
    }

    return err

}

//
// UpdateWeWorkEmployeeRequest
//  @Description:
//  @receiver this
//  @param ctx
//  @param dep
//  @return err
//
func (this *wechatUseCase) UpdateWeWorkEmployeeRequest(ctx context.Context, employee *organization.WeWorkEmployee) (err error) {

    update, err := this.wework.User.Update(ctx, this.employeeModelToWeWorkRequest(employee))

    if err != nil {
        panic(err)
    } else {
        err = this.help.error(`scrm.update.wework.organization.employee.error`, *update)
    }

    if err == nil {
        this.modelWeworkOrganization.employee.Action(this.db, []*organization.WeWorkEmployee{employee})
    }
    return err

}

//
// employeeModelToWeWorkRequest
//  @Description:
//  @param employee
//  @return *request.RequestUserDetail
//
func (this *wechatUseCase) employeeModelToWeWorkRequest(employee *organization.WeWorkEmployee) *request.RequestUserDetail {

    return &request.RequestUserDetail{
        Userid:         employee.WeWorkUserId,
        Name:           employee.Name,
        Alias:          employee.Alias,
        Mobile:         employee.Mobile,
        Position:       employee.Position,
        Email:          employee.Email,
        BizMail:        employee.BizMail,
        Telephone:      employee.Telephone,
        Address:        employee.Address,
        MainDepartment: employee.WeWorkMainDepartmentId,
    }
}

//
// PullSyncDepartmentsAndEmployeesRequest
//  @Description:
//  @receiver uc
//  @param ctx
//  @return error
//
func (this *wechatUseCase) PullSyncDepartmentsAndEmployeesRequest(ctx context.Context) error {

    list, err := this.wework.Department.SimpleList(ctx, 1)
    if err != nil {
        panic(err)
    } else {
        err = this.help.error(`scrm.pull.wework.sync.organization.list.error`, list.ResponseWork)
    }

    if err != nil {
        return err
    }

    this.gLock.Add(len(list.DepartmentIDs))
    for _, val := range list.DepartmentIDs {
        go func(val response.DepartmentID) {
            defer this.gLock.Done()
            this.deparment(val)
            this.employee(val)

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

    department, err := this.wework.Department.Get(this.ctx, val.ID)
    if err != nil {
        panic(err)
    } else {
        err = this.help.error(`scrm.wechat.sync.organization.department.error`, department.ResponseWork)
    }

    if err == nil && department.Department != nil {

        this.modelWeworkOrganization.department.Action(this.db, []*organization.WeWorkDepartment{
            {
                WeWorkDepId:      department.Department.ID,
                Name:             department.Department.Name,
                NameEn:           department.Department.NameEN,
                WeWorkParentId:   department.Department.ParentID,
                Order:            department.Department.Order,
                DepartmentLeader: strings.Join(department.Department.DepartmentLeaders, `,`),
            },
        })

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
        panic(err)
    } else {
        err = this.help.error(`scrm.wework.sync.organization.user.error`, users.ResponseWork)
    }

    if err == nil {
        employees := []*organization.WeWorkEmployee{}
        for _, user := range users.UserList {
            if user != nil {
                open, _ := this.wework.User.UserIdToOpenID(this.ctx, user.UserID)
                employees = append(employees, &organization.WeWorkEmployee{
                    WeWorkUserId:           user.UserID,
                    Name:                   user.Name,
                    Position:               user.Position,
                    Mobile:                 user.UserID,
                    Email:                  fmt.Sprintf(`%s@todo.com`, user.UserID),
                    Alias:                  user.Alias,
                    OpenUserId:             open.OpenID,
                    WeWorkMainDepartmentId: user.MainDepartment,
                    Status:                 user.Status,
                    QrCode:                 user.QrCode,
                    RefEmployeeId:          0,
                })
            }
        }
        this.modelWeworkOrganization.employee.Action(this.db, employees)
        // sync to local
        this.modelOrganization.employee.Action(this.db, this.employeeFromWeWorkSyncToLocal(employees))

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
func (this *wechatUseCase) FindManyWechatEmployeesPage(ctx context.Context, opt *types.PageOption[FindManyWechatEmployeesOption]) (*types.Page[*organization.WeWorkEmployee], error) {

    var employees []*organization.WeWorkEmployee
    var count int64

    query := this.db.WithContext(ctx).Table(this.modelWeworkOrganization.employee.TableName())

    if opt.PageIndex == 0 {
        opt.PageIndex = 1
    }
    if opt.PageSize == 0 {
        opt.PageSize = powermodel.PageDefaultSize
    }
    query = buildFindManyEmployeesQueryNoPage(query, &opt.Option)

    if err := query.Count(&count).Error; err != nil {
        return nil, err
    }
    if opt.PageIndex != 0 && opt.PageSize != 0 {
        query.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
    }

    err := query.Find(&employees).Error

    return &types.Page[*organization.WeWorkEmployee]{
        List:      employees,
        PageIndex: opt.PageIndex,
        PageSize:  opt.PageSize,
        Total:     count,
    }, err
}

//
// getWechatEmployeeIDs
//  @Description:
//  @receiver uc
//  @param ctx
//  @param opt
//  @return *types.Page[*organization.WeWorkEmployee]
//  @return error
//
func (this *wechatUseCase) getWechatEmployeeIDs(ctx context.Context) (ids []string, err error) {

    ids = organization.AdapterEmployeeSliceUserIDs(func(employees []*organization.WeWorkEmployee) (ids []string) {
        for _, employee := range employees {
            ids = append(ids, employee.WeWorkUserId)
        }
        return ids
    })(this.modelWeworkOrganization.employee.Query(this.db))

    return ids, err

}

//
// employeeFromWeWorkSyncToLocal
//  @Description:
//  @receiver this
//  @param fromEmployee
//  @return toEmployee
//
func (this *wechatUseCase) employeeFromWeWorkSyncToLocal(fromEmployee []*organization.WeWorkEmployee) (toEmployee []*organization.Employee) {

    if fromEmployee != nil {
        password, _ := organization.HashPassword(`123456`)
        for _, employee := range fromEmployee {
            toEmployee = append(toEmployee, &organization.Employee{
                Account:       employee.WeWorkUserId,
                Name:          employee.Name,
                NickName:      employee.Name,
                Position:      employee.Position,
                DepartmentId:  int64(employee.WeWorkMainDepartmentId),
                MobilePhone:   employee.Mobile,
                Gender:        employee.Gender,
                Email:         employee.Email,
                ExternalEmail: employee.Email,
                Avatar:        employee.Avatar,
                Password:      password,
                WeWorkUserId:  employee.WeWorkUserId,
            })
        }
    }

    return toEmployee
}
