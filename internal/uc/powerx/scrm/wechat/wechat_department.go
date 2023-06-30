package wechat

import (
    "PowerX/internal/model/scrm/organization"
    "PowerX/internal/types"
    "context"
)

//
// CreateWechatDepartment
//  @Description:
//  @receiver uc
//  @param ctx
//  @param dep
//  @return error
//
func (uc *wechatUseCase) CreateWechatDepartment(ctx context.Context, dep *organization.WeWorkDepartment) (err error) {

    return err
}

//
// FindManyWechatDepartmentsPage
//  @Description:
//  @receiver uc
//  @param ctx
//  @param option
//  @return *types.Page[*organization.WeWorkDepartment]
//  @return error
//
func (uc *wechatUseCase) FindManyWechatDepartmentsPage(ctx context.Context, option *types.PageOption[FindManyWechatDepartmentsOption]) (*types.Page[*organization.WeWorkDepartment], error) {

    var deps []*organization.WeWorkDepartment
    var count int64
    query := uc.db.WithContext(ctx).Model(organization.WeWorkDepartment{})

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
func (uc *wechatUseCase) FindAllWechatDepartments(ctx context.Context) (departments []*organization.WeWorkDepartment, err error) {

    err = uc.db.WithContext(ctx).Find(&departments).Error
    return departments, err

}
