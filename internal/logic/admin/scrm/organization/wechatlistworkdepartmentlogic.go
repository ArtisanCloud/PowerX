package organization

import (
    "PowerX/internal/model/scrm/organization"
    "PowerX/internal/uc/powerx/scrm/wechat"
    "context"
    "strings"

    "PowerX/internal/svc"
    "PowerX/internal/types"

    "github.com/zeromicro/go-zero/core/logx"
)

type WechatListWorkDepartMentLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewWechatListWorkDepartMentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *WechatListWorkDepartMentLogic {
    return &WechatListWorkDepartMentLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// WechatListWorkDepartMent
//  @Description:
//  @receiver this
//  @param req
//  @return resp
//  @return err
//
func (this *WechatListWorkDepartMentLogic) WechatListWorkDepartMent(req *types.ListWechatWorkDepartmentReqeust) (resp *types.ListWechatWorkDepartmentReply, err error) {

    data, err := this.svcCtx.PowerX.SCRM.Wechat.FindManyWechatDepartmentsPage(this.ctx, this.OPT(req))

    return &types.ListWechatWorkDepartmentReply{
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
//  @return *types.PageOption[wechat.FindManyWechatDepartmentsOption]
//
func (this *WechatListWorkDepartMentLogic) OPT(opt *types.ListWechatWorkDepartmentReqeust) *types.PageOption[wechat.FindManyWechatDepartmentsOption] {

    option := types.PageOption[wechat.FindManyWechatDepartmentsOption]{
        Option:    wechat.FindManyWechatDepartmentsOption{},
        PageIndex: opt.PageIndex,
        PageSize:  opt.PageSize,
    }
    if v := opt.WeWorkParentId; v > 0 {
        option.Option.WeWorkDepId = []int{v}
    }
    if v := opt.Name; v != `` {
        option.Option.Name = v
    }
    option.DefaultPageIfNotSet()

    return &option

}

//
// DTO
//  @Description:
//  @receiver this
//  @param data
//  @return departments
//
func (this *WechatListWorkDepartMentLogic) DTO(data []*organization.WeWorkDepartment) (departments []*types.WechatDepartment) {

    for _, val := range data {
        departments = append(departments, this.dto(val))
    }
    return departments

}

//
// dto
//  @Description:
//  @receiver this
//  @param val
//  @return *types.WechatDepartment
//
func (this *WechatListWorkDepartMentLogic) dto(val *organization.WeWorkDepartment) *types.WechatDepartment {
    var leader []string
    if val.DepartmentLeader != `` {
        leader = strings.Split(val.DepartmentLeader, `,`)
    }
    return &types.WechatDepartment{
        WeWorkDepId:      val.WeWorkDepId,
        Name:             val.Name,
        NameEn:           val.NameEn,
        WeWorkParentId:   val.WeWorkParentId,
        Order:            val.Order,
        RefDepartmentId:  val.RefDepartmentId,
        DepartmentLeader: leader,
    }
}
