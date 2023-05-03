package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListArtisanSpecificsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListArtisanSpecificsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListArtisanSpecificsLogic {
	return &ListArtisanSpecificsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListArtisanSpecificsLogic) ListArtisanSpecifics(req *types.GetArtisanSpecificListRequest) (resp *types.GetArtisanSpecificListReply, err error) {
	// todo: add your logic here and delete this line

	return
}
