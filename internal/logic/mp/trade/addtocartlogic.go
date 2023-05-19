package trade

import (
	"PowerX/internal/uc/powerx/customerdomain"
	fmt "PowerX/pkg/printx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AddToCartLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAddToCartLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddToCartLogic {
	return &AddToCartLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AddToCartLogic) AddToCart(req *types.AddToCartRequest) (resp *types.AddToCartReply, err error) {

	authUser := l.ctx.Value(customerdomain.AuthCustomerKey)
	openID := l.ctx.Value(customerdomain.AuthCustomerOpenIdKey)
	fmt.Dump(authUser, openID)

	return
}
