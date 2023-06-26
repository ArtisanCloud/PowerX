package productspecific

import (
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteProductSpecificLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteProductSpecificLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteProductSpecificLogic {
	return &DeleteProductSpecificLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteProductSpecificLogic) DeleteProductSpecific(req *types.DeleteProductSpecificRequest) (resp *types.DeleteProductSpecificReply, err error) {
	productSpecific, err := l.svcCtx.PowerX.ProductSpecific.GetProductSpecific(l.ctx, req.ProductSpecificId)
	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}
	//if len(productSpecific.Options) > 0 {
	//	return nil, errorx.WithCause(errorx.ErrBadRequest, "该品类包含子选项，请处理选项")
	//}

	err = l.svcCtx.PowerX.ProductSpecific.DeleteProductSpecific(l.ctx, productSpecific.Id)
	if err != nil {
		return nil, err
	}

	return &types.DeleteProductSpecificReply{
		ProductSpecificId: productSpecific.Id,
	}, nil
}
