package tag

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

type SyncWeWorkGroupTagLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSyncWeWorkGroupTagLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SyncWeWorkGroupTagLogic {
	return &SyncWeWorkGroupTagLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

//
// SyncWeWorkGroupTag
//  @Description: 全量同步标签
//  @receiver tag
//  @return resp
//  @return err
//
func (tag *SyncWeWorkGroupTagLogic) SyncWeWorkGroupTag() (resp *types.StatusWeWorkReply, err error) {

	_, err = tag.svcCtx.PowerX.SCRM.Wechat.PullListWeWorkCorpTagRequest(nil, nil, 1)

	return &types.StatusWeWorkReply{
		Status: `success`,
	}, err
}
