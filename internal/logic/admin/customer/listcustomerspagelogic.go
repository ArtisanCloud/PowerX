package customer

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListCustomersPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListCustomersPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListCustomersPageLogic {
	return &ListCustomersPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListCustomersPageLogic) ListCustomersPage(req *types.ListCustomersPageRequest) (resp *types.ListCustomersPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
