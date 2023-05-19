package opportunity

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteOpportunityLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteOpportunityLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteOpportunityLogic {
	return &DeleteOpportunityLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteOpportunityLogic) DeleteOpportunity(req *types.DeleteOpportunityRequest) (resp *types.DeleteOpportunityReply, err error) {
	// todo: add your logic here and delete this line

	return
}
