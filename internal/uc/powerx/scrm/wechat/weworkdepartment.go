package wechat

import (
    "PowerX/internal/model/scrm/organization"
    "PowerX/internal/types"
    "context"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/department/request"
)

//
// CreateWeWorkDepartmentRequest
//  @Description:
//  @receiver uc
//  @param ctx
//  @param dep
//  @return error
//
func (this *wechatUseCase) CreateWeWorkDepartmentRequest(ctx context.Context, dep *organization.WeWorkDepartment) (err error) {

    create, err := this.wework.Department.Create(ctx, &request.RequestDepartmentInsert{
        Name:     dep.Name,
        NameEn:   dep.NameEn,
        ParentID: dep.WeWorkParentId,
        Order:    dep.Order,
        ID:       int(dep.Id),
    })

    if err != nil {
        panic(err)
    } else {
        err = this.help.error(`scrm.create.wework.department.error`, create.ResponseWork)

    }
    return err

}

//
// UpdateWeWorkDepartmentRequest
//  @Description:
//  @receiver this
//  @param ctx
//  @param dep
//  @return err
//
func (this *wechatUseCase) UpdateWeWorkDepartmentRequest(ctx context.Context, dep *organization.WeWorkDepartment) (err error) {

    update, err := this.wework.Department.Update(ctx, &request.RequestDepartmentUpdate{
        Name:     dep.Name,
        NameEn:   dep.NameEn,
        ParentID: dep.WeWorkParentId,
        Order:    dep.Order,
        ID:       int(dep.Id),
    })
    if err != nil {
        panic(err)
    } else {
        err = this.help.error(`scrm.update.wework.department.error`, update.ResponseWork)

    }

    return err

}

//
// FindManyWeWorkDepartmentsPage
//  @Description:
//  @receiver uc
//  @param ctx
//  @param option
//  @return *types.Page[*organization.WeWorkDepartment]
//  @return error
//
func (this *wechatUseCase) FindManyWeWorkDepartmentsPage(ctx context.Context, option *types.PageOption[FindManyWechatDepartmentsOption]) (*types.Page[*organization.WeWorkDepartment], error) {

    var deps []*organization.WeWorkDepartment
    var count int64
    query := this.db.WithContext(ctx).Model(organization.WeWorkDepartment{})

    if len(option.Option.WeWorkDepId) > 0 {
        query.Where(`we_work_dep_id in ?`, option.Option.WeWorkDepId)
    }

    if v := option.Option.Name; v == `` {
        query.Where("name like ?", "%"+v+"%")
    }

    if err := query.Count(&count).Error; err != nil {
        return nil, err
    }
    if option.PageIndex != 0 && option.PageSize != 0 {
        query.Offset((option.PageIndex - 1) * option.PageSize).Limit(option.PageSize)
    }
    err := query.Find(&deps).Error

    return &types.Page[*organization.WeWorkDepartment]{
        List:      deps,
        PageIndex: option.PageIndex,
        PageSize:  option.PageSize,
        Total:     count,
    }, err

}

//
// FindAllWechatDepartments
//  @Description:
//  @receiver uc
//  @param ctx
//  @return departments
//  @return err
//
func (this *wechatUseCase) FindAllWechatDepartments(ctx context.Context) (departments []*organization.WeWorkDepartment, err error) {

    err = this.db.WithContext(ctx).Find(&departments).Error
    return departments, err

}
