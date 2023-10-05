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
	// todo: add your logic here and delete this line

	return
}
