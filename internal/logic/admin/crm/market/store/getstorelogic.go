package store

import (
	product2 "PowerX/internal/model/crm/product"
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetStoreLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetStoreLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetStoreLogic {
	return &GetStoreLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetStoreLogic) GetStore(req *types.GetStoreRequest) (resp *types.GetStoreReply, err error) {
	mdlStore, err := l.svcCtx.PowerX.Store.GetStore(l.ctx, req.StoreId)

	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}

	return &types.GetStoreReply{
		Store: TransformStoreToReply(mdlStore),
	}, nil

}

func TransformArtisansToShopArtisans(artisans []*product2.Artisan) []*types.StoreArtisan {
	artisansReply := []*types.StoreArtisan{}
	for _, artisan := range artisans {
		artisanReply := TransformArtisanToShopArtisan(artisan)
		artisansReply = append(artisansReply, artisanReply)
	}
	return artisansReply
}

func TransformArtisanToShopArtisan(artisan *product2.Artisan) *types.StoreArtisan {
	return &types.StoreArtisan{
		UserId:      artisan.UserId,
		Name:        artisan.Name,
		Level:       artisan.Level,
		Gender:      artisan.Gender,
		Birthday:    artisan.Birthday.String(),
		PhoneNumber: artisan.PhoneNumber,
		WorkNo:      artisan.WorkNo,
		Email:       artisan.Email,
		Experience:  artisan.Experience,
		Specialty:   artisan.Specialty,
		Certificate: artisan.Certificate,
		Address:     artisan.Address,
	}
}
