package lead

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListLeadsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListLeadsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListLeadsLogic {
	return &ListLeadsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListLeadsLogic) ListLeads(req *types.ListLeadsRequest) (resp *types.ListLeadsReply, err error) {
	// todo: add your logic here and delete this line

	return
}
