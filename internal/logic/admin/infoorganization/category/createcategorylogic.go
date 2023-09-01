package category

import (
	"PowerX/internal/model"
	infoorganizatoin "PowerX/internal/model/infoorganization"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateCategoryLogic {
	return &CreateCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateCategoryLogic) CreateCategory(req *types.CreateCategoryRequest) (resp *types.CreateCategoryReply, err error) {
	productCategory := TransformRequestToCategory(&req.Category)

	productCategory, err = l.svcCtx.PowerX.Category.UpsertCategory(l.ctx, productCategory)
	if err != nil {
		return nil, err
	}

	return &types.CreateCategoryReply{
		Category: TransformCategoryToReply(productCategory),
	}, nil
}

func TransformRequestToCategory(req *types.Category) *infoorganizatoin.Category {
	return &infoorganizatoin.Category{
		PId:          req.PId,
		Name:         req.Name,
		Sort:         req.Sort,
		ViceName:     req.ViceName,
		Description:  req.Description,
		CoverImageId: req.CoverImageId,
		ImageAbleInfo: model.ImageAbleInfo{
			Icon:            req.Icon,
			BackgroundColor: req.BackgroundColor,
		},
	}
}
