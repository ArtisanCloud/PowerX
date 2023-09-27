package opportunity

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AssignEmployeeToOpportunityLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignEmployeeToOpportunityLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignEmployeeToOpportunityLogic {
	return &AssignEmployeeToOpportunityLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AssignEmployeeToOpportunityLogic) AssignEmployeeToOpportunity(req *types.AssignEmployeeToOpportunityRequest) (resp *types.AssignEmployeeToOpportunityReply, err error) {
	// todo: add your logic here and delete this line

	return
}
