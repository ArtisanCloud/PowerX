package shipping

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListShippingAddressesPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListShippingAddressesPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListShippingAddressesPageLogic {
	return &ListShippingAddressesPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListShippingAddressesPageLogic) ListShippingAddressesPage(req *types.ListShippingAddressesPageRequest) (resp *types.ListShippingAddressesPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
