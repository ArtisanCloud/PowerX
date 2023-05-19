package category

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchProductCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchProductCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchProductCategoryLogic {
	return &PatchProductCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchProductCategoryLogic) PatchProductCategory(req *types.PatchProductCategoryRequest) (resp *types.PatchProductCategoryReply, err error) {
	// todo: add your logic here and delete this line

	return
}
