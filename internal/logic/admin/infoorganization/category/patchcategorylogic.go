package category

import (
	infoorganizatoin "PowerX/internal/model/infoorganization"
	"PowerX/internal/model/powermodel"
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

	productCategory := &infoorganizatoin.Category{
		PowerModel: powermodel.PowerModel{
			Id: req.Id,
		},
		PId: req.PId,
	}

	l.svcCtx.PowerX.Category.PatchCategory(l.ctx, req.Id, productCategory)

	return &types.PatchCategoryReply{
		Category: types.Category{
			Id:          productCategory.Id,
			PId:         productCategory.PId,
			Name:        productCategory.Name,
			Sort:        productCategory.Sort,
			ViceName:    productCategory.ViceName,
			Description: productCategory.Description,
			CreatedAt:   productCategory.CreatedAt.String(),
			ImageAbleInfo: types.ImageAbleInfo{
				Icon:            productCategory.Icon,
				BackgroundColor: productCategory.BackgroundColor,
			},
		},
	}, nil

}
