package lead

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetLeadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetLeadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetLeadLogic {
	return &GetLeadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetLeadLogic) GetLead(req *types.GetLeadReqeuest) (resp *types.GetLeadReply, err error) {
	// todo: add your logic here and delete this line

	return
}
