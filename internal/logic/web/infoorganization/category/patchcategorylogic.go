package category

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchCategoryLogic {
	return &PatchCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchCategoryLogic) PatchCategory(req *types.PatchCategoryRequest) (resp *types.PatchCategoryReply, err error) {
	// todo: add your logic here and delete this line

	return
}
