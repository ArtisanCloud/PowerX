package opportunity

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateOpportunityLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateOpportunityLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateOpportunityLogic {
	return &UpdateOpportunityLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateOpportunityLogic) UpdateOpportunity(req *types.UpdateOpportunityRequest) (resp *types.UpdateOpportunityReply, err error) {
	// todo: add your logic here and delete this line

	return
}
