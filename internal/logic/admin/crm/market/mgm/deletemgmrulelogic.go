package mgm

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteMGMRuleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteMGMRuleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteMGMRuleLogic {
	return &DeleteMGMRuleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteMGMRuleLogic) DeleteMGMRule(req *types.DeleteMGMRuleRequest) (resp *types.DeleteMGMRuleReply, err error) {
	err = l.svcCtx.PowerX.MGM.DeleteMGMRule(l.ctx, req.MGMRuleId)
	if err != nil {
		return nil, err
	}

	return &types.DeleteMGMRuleReply{
		MGMRuleId: req.MGMRuleId,
	}, nil
}
