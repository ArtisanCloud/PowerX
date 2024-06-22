package artisan

import (
	"PowerX/internal/logic/admin/mediaresource"
	"PowerX/internal/model/crm/product"
	"PowerX/internal/model/media"
	product3 "PowerX/internal/uc/powerx/crm/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListArtisansPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListArtisansPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListArtisansPageLogic {
	return &ListArtisansPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListArtisansPageLogic) ListArtisansPage(req *types.ListArtisansPageRequest) (resp *types.ListArtisansPageReply, err error) {
	artisans, err := l.svcCtx.PowerX.Artisan.FindManyArtisans(l.ctx, &product3.FindManyArtisanOption{
		LikeName: req.LikeName,
		OrderBy:  req.OrderBy,
		StoreIds: req.StoreIds,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})

	if err != nil {
		return nil, err
	}
	list := TransformArtisansToReply(artisans.List)

	return &types.ListArtisansPageReply{
		List:      list,
		PageIndex: artisans.PageIndex,
		PageSize:  artisans.PageSize,
		Total:     artisans.Total,
	}, nil
}

func TransformArtisansToReply(artisans []*product.Artisan) []*types.Artisan {
	artisansReply := []*types.Artisan{}
	for _, artisan := range artisans {
		artisanReply := TransformArtisanToReply(artisan)
		artisansReply = append(artisansReply, artisanReply)
	}
	return artisansReply
}

func TransformArtisanToReply(artisan *product.Artisan) *types.Artisan {
	arrayDetailImageIds, _ := media.GetImageIds(artisan.PivotDetailImages)
	return &types.Artisan{
		Id:             artisan.Id,
		UserId:         artisan.UserId,
		Name:           artisan.Name,
		Level:          artisan.Level,
		Gender:         artisan.Gender,
		Birthday:       artisan.Birthday.String(),
		PhoneNumber:    artisan.PhoneNumber,
		WorkNo:         artisan.WorkNo,
		Email:          artisan.Email,
		Experience:     artisan.Experience,
		Specialty:      artisan.Specialty,
		Certificate:    artisan.Certificate,
		Address:        artisan.Address,
		CreatedAt:      artisan.CreatedAt.String(),
		CoverImageId:   artisan.CoverImageId,
		CoverImage:     mediaresource.TransformMediaResourceToReply(artisan.CoverImage),
		DetailImageIds: arrayDetailImageIds,
		DetailImages:   mediaresource.TransformMediaResourcesToReply(artisan.PivotDetailImages),
		StoreIds:       product.GetStoreIds(artisan.PivotStoreToArtisans),
	}
}
