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
	// todo: add your logic here and delete this line

	return
}
