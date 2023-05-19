package category

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpsertProductCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpsertProductCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpsertProductCategoryLogic {
	return &UpsertProductCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpsertProductCategoryLogic) UpsertProductCategory(req *types.UpsertProductCategoryRequest) (resp *types.UpsertProductCategoryReply, err error) {
	// todo: add your logic here and delete this line

	return
}
