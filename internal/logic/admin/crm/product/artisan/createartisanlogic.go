package artisan

import (
	"PowerX/internal/model/crm/product"
	"PowerX/internal/model/media"
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/golang-module/carbon/v2"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateArtisanLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateArtisanLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateArtisanLogic {
	return &CreateArtisanLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateArtisanLogic) CreateArtisan(req *types.CreateArtisanRequest) (resp *types.CreateArtisanReply, err error) {
	mdlArtisan := TransformRequestToArtisan(&req.Artisan)

	if len(req.DetailImageIds) > 0 {
		mediaResources, err := l.svcCtx.PowerX.MediaResource.FindAllMediaResources(l.ctx, &powerx.FindManyMediaResourcesOption{
			Ids: req.DetailImageIds,
		})
		if err != nil {
			return nil, err
		}
		mdlArtisan.PivotDetailImages, err = (&media.PivotMediaResourceToObject{}).MakeMorphPivotsFromObjectToMediaResources(mdlArtisan, mediaResources, media.MediaUsageDetail)
	}

	err = l.svcCtx.PowerX.Artisan.CreateArtisan(l.ctx, mdlArtisan)

	return &types.CreateArtisanReply{
		ArtisanId: mdlArtisan.Id,
	}, nil
}

func TransformRequestToArtisan(artisanRequest *types.Artisan) (mdlArtisan *product.Artisan) {

	birthday := carbon.Parse(artisanRequest.Birthday)

	return &product.Artisan{
		EmployeeId:   artisanRequest.EmployeeId,
		Name:         artisanRequest.Name,
		Level:        artisanRequest.Level,
		Gender:       artisanRequest.Gender,
		Birthday:     birthday.ToStdTime(),
		PhoneNumber:  artisanRequest.PhoneNumber,
		CoverImageId: artisanRequest.CoverImageId,
		WorkNo:       artisanRequest.WorkNo,
		Email:        artisanRequest.Email,
		Experience:   artisanRequest.Experience,
		Specialty:    artisanRequest.Specialty,
		Certificate:  artisanRequest.Certificate,
		Address:      artisanRequest.Address,
	}
}
