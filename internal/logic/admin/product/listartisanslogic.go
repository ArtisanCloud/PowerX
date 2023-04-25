package product

import (
	product2 "PowerX/internal/model/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListArtisansLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListArtisansLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListArtisansLogic {
	return &ListArtisansLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListArtisansLogic) ListArtisans(req *types.GetArtisanListRequest) (resp *types.GetArtisanListReply, err error) {
	// todo: add your logic here and delete this line

	return
}

func TransferArtisansToArtisansReply(artisans []*product2.Artisan) []*types.StoreArtisan {
	artisansReply := []*types.StoreArtisan{}
	for _, artisan := range artisans {
		artisanReply := TransferArtisanToShopArtisan(artisan)
		artisansReply = append(artisansReply, artisanReply)
	}
	return artisansReply
}
