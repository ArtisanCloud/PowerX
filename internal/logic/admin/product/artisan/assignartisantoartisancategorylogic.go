package artisan

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AssignArtisanToArtisanCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignArtisanToArtisanCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignArtisanToArtisanCategoryLogic {
	return &AssignArtisanToArtisanCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AssignArtisanToArtisanCategoryLogic) AssignArtisanToArtisanCategory(req *types.AssignArtisanManagerRequest) (resp *types.AssignArtisanManagerReply, err error) {
	// todo: add your logic here and delete this line

	return
}
