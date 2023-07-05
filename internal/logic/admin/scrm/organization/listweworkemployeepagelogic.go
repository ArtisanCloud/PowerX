package organization

import (
    "PowerX/internal/model/scrm/organization"
    "PowerX/internal/uc/powerx/scrm/wechat"
    "context"

    "PowerX/internal/svc"
    "PowerX/internal/types"

    "github.com/zeromicro/go-zero/core/logx"
)

type ListWeWorkEmployeePageLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewListWeWorkEmployeePageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWeWorkEmployeePageLogic {
    return &ListWeWorkEmployeePageLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// ListWeWorkEmployeePage
//  @Description:
//  @receiver this
//  @param opt
//  @return resp
//  @return err
//
func (this *ListWeWorkEmployeePageLogic) ListWeWorkEmployeePage(opt *types.ListWeWorkEmployeeReqeust) (resp *types.ListWeWorkEmployeeReply, err error) {
    data, err := this.svcCtx.PowerX.SCRM.Wechat.FindManyWechatEmployeesPage(this.ctx, this.OPT(opt))

    return &types.ListWeWorkEmployeeReply{
        List:      this.DTO(data.List),
        PageIndex: data.PageIndex,
        PageSize:  data.PageSize,
        Total:     data.Total,
    }, err
}

//
// OPT
//  @Description:
//  @receiver this
//  @param opt
//  @return *types.PageOption[wechat.FindManyWechatEmployeesOption]
//
func (this *ListWeWorkEmployeePageLogic) OPT(opt *types.ListWeWorkEmployeeReqeust) *types.PageOption[wechat.FindManyWechatEmployeesOption] {

    option := types.PageOption[wechat.FindManyWechatEmployeesOption]{
        Option:    wechat.FindManyWechatEmployeesOption{},
        PageIndex: opt.PageIndex,
        PageSize:  opt.PageSize,
    }
    if opt.Id > 0 {
        option.Option.Ids = []int64{opt.Id}
    }
    if opt.Name != `` {
        option.Option.Names = []string{opt.Name}
    }
    if opt.Alias != `` {
        option.Option.Alias = []string{opt.Alias}
    }
    if opt.Email != `` {
        option.Option.Emails = []string{opt.Email}
    }
    if opt.Mobile != `` {
        option.Option.Mobile = []string{opt.Mobile}
    }
    if opt.OpenUserId != `` {
        option.Option.OpenUserId = []string{opt.OpenUserId}
    }
    if opt.WeWorkMainDepartmentId > 0 {
        option.Option.WeWorkMainDepartmentId = []int64{opt.WeWorkMainDepartmentId}
    }
    if opt.Status > 0 {
        option.Option.Status = []int{opt.Status}
    }
    option.DefaultPageIfNotSet()

    return &option

}

//
// DTO
//  @Description:
//  @receiver this
//  @param data
//  @return employees
//
func (this *ListWeWorkEmployeePageLogic) DTO(data []*organization.WeWorkEmployee) (employees []*types.WechatEmployee) {

    for _, val := range data {
        employees = append(employees, this.dto(val))
    }
    return employees

}

//
// dto
//  @Description:
//  @receiver this
//  @param val
//  @return *types.WechatEmployee
//
func (this *ListWeWorkEmployeePageLogic) dto(val *organization.WeWorkEmployee) *types.WechatEmployee {
    return &types.WechatEmployee{
        WeWorkUserId:           val.WeWorkUserId,
        Name:                   val.Name,
        Position:               val.Position,
        Mobile:                 val.Mobile,
        Gender:                 val.Gender,
        Email:                  val.Email,
        BizMail:                val.BizMail,
        Avatar:                 val.Avatar,
        ThumbAvatar:            val.ThumbAvatar,
        Telephone:              val.Telephone,
        Alias:                  val.Alias,
        Address:                val.Address,
        OpenUserId:             val.OpenUserId,
        WeWorkMainDepartmentId: val.WeWorkMainDepartmentId,
        Status:                 val.Status,
        QrCode:                 val.QrCode,
    }
}
