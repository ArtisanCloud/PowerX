package mgm

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateMGMRuleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateMGMRuleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateMGMRuleLogic {
	return &UpdateMGMRuleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateMGMRuleLogic) UpdateMGMRule(req *types.UpdateMGMRuleRequest) (resp *types.UpdateMGMRuleReply, err error) {
	mdlMGMRule := TransformRequestToMGMRule(&(req.MGMRule))
	mdlMGMRule.Id = req.MGMRuleId

	// 更新MGM对象
	mdlMGMRule, err = l.svcCtx.PowerX.MGM.UpsertMGMRule(l.ctx, mdlMGMRule)
	if err != nil {
		return nil, err
	}

	return &types.UpdateMGMRuleReply{
		MGMRuleId: mdlMGMRule.Id,
	}, nil
}
