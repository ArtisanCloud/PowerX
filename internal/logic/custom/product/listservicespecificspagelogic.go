package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListServiceSpecificsPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListServiceSpecificsPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListServiceSpecificsPageLogic {
	return &ListServiceSpecificsPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListServiceSpecificsPageLogic) ListServiceSpecificsPage(req *types.ListServiceSpecificPageRequest) (resp *types.ListServiceSpecificPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
