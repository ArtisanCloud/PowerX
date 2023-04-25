package product

import (
	product2 "PowerX/internal/model/product"
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetArtisanLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetArtisanLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetArtisanLogic {
	return &GetArtisanLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetArtisanLogic) GetArtisan(req *types.GetArtisanRequest) (resp *types.GetArtisanReply, err error) {
	mdlArtisan, err := l.svcCtx.PowerX.Artisan.GetArtisan(l.ctx, req.ArtisanId)

	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}

	return &types.GetArtisanReply{
		Artisan: TransferArtisanToArtisanReply(mdlArtisan),
	}, nil

}

func TransferArtisanToArtisanReply(artisan *product2.Artisan) *types.Artisan {
	return &types.Artisan{
		Id:          artisan.Id,
		EmployeeId:  artisan.EmployeeId,
		Name:        artisan.Name,
		Level:       artisan.Level,
		Gender:      artisan.Gender,
		Birthday:    artisan.Birthday.String(),
		PhoneNumber: artisan.PhoneNumber,
		CoverURL:    artisan.CoverURL,
		WorkNo:      artisan.WorkNo,
		Email:       artisan.Email,
		Experience:  artisan.Experience,
		Specialty:   artisan.Specialty,
		Certificate: artisan.Certificate,
		Address:     artisan.Address,
	}
}
