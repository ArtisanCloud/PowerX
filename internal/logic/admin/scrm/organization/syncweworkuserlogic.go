package organization

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SyncWeWorkUserLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSyncWeWorkUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SyncWeWorkUserLogic {
	return &SyncWeWorkUserLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// SyncWeWorkUser
//
//	@Description: 更新组织架构
//	@receiver sync
//	@return resp
//	@return err
func (sync *SyncWeWorkUserLogic) SyncWeWorkUser() (resp *types.SyncWeWorkOrganizationReply, err error) {

	err = sync.svcCtx.PowerX.SCRM.Wechat.PullSyncDepartmentsAndUsersRequest(sync.ctx)

	return &types.SyncWeWorkOrganizationReply{
		Status: `success`,
	}, err
}
