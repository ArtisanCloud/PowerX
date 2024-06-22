package opportunity

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AssignUserToOpportunityLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignUserToOpportunityLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignUserToOpportunityLogic {
	return &AssignUserToOpportunityLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AssignUserToOpportunityLogic) AssignUserToOpportunity(req *types.AssignUserToOpportunityRequest) (resp *types.AssignUserToOpportunityReply, err error) {
	// todo: add your logic here and delete this line

	return
}
