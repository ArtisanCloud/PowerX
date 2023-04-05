package opportunity

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateOpportunityLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateOpportunityLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateOpportunityLogic {
	return &CreateOpportunityLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateOpportunityLogic) CreateOpportunity(req *types.CreateOpportunityRequest) (resp *types.CreateOpportunityReply, err error) {
	// todo: add your logic here and delete this line

	return
}
