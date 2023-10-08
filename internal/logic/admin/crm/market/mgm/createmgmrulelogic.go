package mgm

import (
	"PowerX/internal/model/crm/market"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateMGMRuleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateMGMRuleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateMGMRuleLogic {
	return &CreateMGMRuleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateMGMRuleLogic) CreateMGMRule(req *types.CreateMGMRuleRequest) (resp *types.CreateMGMRuleReply, err error) {
	mdlMGMRule := TransformRequestToMGMRule(&req.MGMRule)

	l.svcCtx.PowerX.MGM.CreateMGMRule(l.ctx, mdlMGMRule)

	return &types.CreateMGMRuleReply{
		MGMRuleId: mdlMGMRule.Id,
	}, nil

}

func TransformRequestToMGMRule(mediaRequest *types.MGMRule) (mdlMGMRule *market.MGMRule) {

	return &market.MGMRule{

		Name:            mediaRequest.Name,
		CommissionRate1: mediaRequest.CommissionRate1,
		CommissionRate2: mediaRequest.CommissionRate2,
		Scene:           mediaRequest.Scene,
		Description:     mediaRequest.Description,
	}
}
