package mgm

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListMGMsPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListMGMRulesPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListMGMsPageLogic {
	return &ListMGMsPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListMGMsPageLogic) ListMGMRulesPage(req *types.ListMGMRulesPageRequest) (resp *types.ListMGMRulesPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
