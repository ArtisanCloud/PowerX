package organization

import (
    "context"

    "PowerX/internal/svc"
    "PowerX/internal/types"

    "github.com/zeromicro/go-zero/core/logx"
)

type SyncWeWorkEmployeeLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewSyncWeWorkEmployeeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SyncWeWorkEmployeeLogic {
    return &SyncWeWorkEmployeeLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// SyncWeWorkEmployee
//  @Description: 更新组织架构
//  @receiver sync
//  @return resp
//  @return err
//
func (sync *SyncWeWorkEmployeeLogic) SyncWeWorkEmployee() (resp *types.SyncWeWorkOrganizationReply, err error) {

    err = sync.svcCtx.PowerX.SCRM.Wechat.PullSyncDepartmentsAndEmployeesRequest(sync.ctx)

    return &types.SyncWeWorkOrganizationReply{
        Status: `success`,
    }, err
}
