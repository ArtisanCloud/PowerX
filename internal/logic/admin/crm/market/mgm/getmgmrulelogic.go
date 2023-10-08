package mgm

import (
	"PowerX/internal/model/crm/market"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetMGMRuleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetMGMRuleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMGMRuleLogic {
	return &GetMGMRuleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetMGMRuleLogic) GetMGMRule(req *types.GetMGMRuleRequest) (resp *types.GetMGMRuleReply, err error) {
	// todo: add your logic here and delete this line

	return
}

func TransformMGMRuleToReply(mdlMGMRule *market.MGMRule) (mediaReply *types.MGMRule) {
	return &types.MGMRule{
		Id:              mdlMGMRule.Id,
		Name:            mdlMGMRule.Name,
		CommissionRate1: mdlMGMRule.CommissionRate1,
		CommissionRate2: mdlMGMRule.CommissionRate2,
		Scene:           mdlMGMRule.Scene,
		Description:     mdlMGMRule.Description,
	}
}
