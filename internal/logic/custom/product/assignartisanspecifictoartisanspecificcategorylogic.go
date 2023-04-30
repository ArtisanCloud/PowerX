package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AssignArtisanSpecificToArtisanSpecificCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignArtisanSpecificToArtisanSpecificCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignArtisanSpecificToArtisanSpecificCategoryLogic {
	return &AssignArtisanSpecificToArtisanSpecificCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AssignArtisanSpecificToArtisanSpecificCategoryLogic) AssignArtisanSpecificToArtisanSpecificCategory(req *types.AssignArtisanSpecificManagerRequest) (resp *types.AssignArtisanSpecificManagerReply, err error) {
	// todo: add your logic here and delete this line

	return
}
