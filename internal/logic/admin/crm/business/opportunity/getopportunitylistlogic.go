package opportunity

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetOpportunityListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetOpportunityListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetOpportunityListLogic {
	return &GetOpportunityListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetOpportunityListLogic) GetOpportunityList(req *types.GetOpportunityListRequest) (resp *types.GetOpportunityListReply, err error) {
	// todo: add your logic here and delete this line

	return
}
