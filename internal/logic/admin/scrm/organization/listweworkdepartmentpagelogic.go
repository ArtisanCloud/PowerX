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

type ListWeWorkDepartMentPageLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewListWeWorkDepartMentPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWeWorkDepartMentPageLogic {
    return &ListWeWorkDepartMentPageLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// ListWeWorkDepartMentPage
//  @Description:
//  @receiver this
//  @param opt
//  @return resp
//  @return err
//
func (this *ListWeWorkDepartMentPageLogic) ListWeWorkDepartMentPage(opt *types.ListWeWorkDepartmentReqeust) (resp *types.ListWeWorkDepartmentReply, err error) {

    data, err := this.svcCtx.PowerX.SCRM.Wechat.FindManyWeWorkDepartmentsPage(this.ctx, this.OPT(opt))

    return &types.ListWeWorkDepartmentReply{
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
func (this *ListWeWorkDepartMentPageLogic) OPT(opt *types.ListWeWorkDepartmentReqeust) *types.PageOption[wechat.FindManyWechatDepartmentsOption] {

    option := types.PageOption[wechat.FindManyWechatDepartmentsOption]{
        Option:    wechat.FindManyWechatDepartmentsOption{},
        PageIndex: opt.PageIndex,
        PageSize:  opt.PageSize,
    }
    if v := opt.WeWorkDepId; v > 0 {
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
func (this *ListWeWorkDepartMentPageLogic) DTO(data []*organization.WeWorkDepartment) (departments []*types.WeWorkDepartment) {

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
func (this *ListWeWorkDepartMentPageLogic) dto(val *organization.WeWorkDepartment) *types.WeWorkDepartment {
    var leader []string
    if val.DepartmentLeader != `` {
        leader = strings.Split(val.DepartmentLeader, `,`)
    }
    return &types.WeWorkDepartment{
        WeWorkDepId:      val.WeWorkDepId,
        Name:             val.Name,
        NameEn:           val.NameEn,
        WeWorkParentId:   val.WeWorkParentId,
        Order:            val.Order,
        RefDepartmentId:  val.RefDepartmentId,
        DepartmentLeader: leader,
    }
}
