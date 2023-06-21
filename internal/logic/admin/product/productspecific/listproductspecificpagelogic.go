package productspecific

import (
	product2 "PowerX/internal/model/product"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx/product"
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListProductSpecificPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListProductSpecificPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListProductSpecificPageLogic {
	return &ListProductSpecificPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListProductSpecificPageLogic) ListProductSpecificPage(req *types.ListProductSpecificPageRequest) (resp *types.ListProductSpecificPageReply, err error) {

	if req.ProductId <= 0 {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "请选择产品")
	}

	opt := &product.FindProductSpecificOption{
		ProductId: req.ProductId,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	}

	productSpecificList := l.svcCtx.PowerX.ProductSpecific.FindManyProductSpecifics(l.ctx, opt)
	resp = &types.ListProductSpecificPageReply{}

	resp.List = TransformProductSpecificsToProductSpecificsReply(productSpecificList.List)
	resp.PageIndex = productSpecificList.PageIndex
	resp.PageSize = productSpecificList.PageSize
	resp.Total = productSpecificList.Total

	return resp, nil
}

func TransformProductSpecificsToProductSpecificsReply(specifics []*product2.ProductSpecific) (specificsReply []*types.ProductSpecific) {

	specificsReply = []*types.ProductSpecific{}
	for _, entry := range specifics {
		specificsReply = append(specificsReply, TransformProductSpecificToProductSpecificReply(entry))

	}

	return specificsReply
}
