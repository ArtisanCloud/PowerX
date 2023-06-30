package organization

import (
    "context"

    "PowerX/internal/svc"
    "PowerX/internal/types"

    "github.com/zeromicro/go-zero/core/logx"
)

type SyncWechatWorkEmployeeLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewWechatSyncWorkEmployeeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SyncWechatWorkEmployeeLogic {
    return &SyncWechatWorkEmployeeLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// WechatSyncWorkEmployee
//  @Description:
//  @receiver this
//  @return resp
//  @return err
//
func (this *SyncWechatWorkEmployeeLogic) WechatSyncWorkEmployee() (resp *types.SyncWechatWorkOrganizationReply, err error) {

    err = this.svcCtx.PowerX.SCRM.Wechat.SyncDepartmentsAndEmployees(this.ctx)

    return &types.SyncWechatWorkOrganizationReply{
        Status: `success`,
    }, err
}
