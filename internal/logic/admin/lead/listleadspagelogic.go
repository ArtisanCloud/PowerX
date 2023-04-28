package lead

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListLeadsPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListLeadsPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListLeadsPageLogic {
	return &ListLeadsPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListLeadsPageLogic) ListLeadsPage(req *types.ListLeadsPageRequest) (resp *types.ListLeadsPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
