package trade

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListDeliveryAddressesPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListDeliveryAddressesPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListDeliveryAddressesPageLogic {
	return &ListDeliveryAddressesPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListDeliveryAddressesPageLogic) ListDeliveryAddressesPage(req *types.ListDeliveryAddressesPageRequest) (resp *types.ListDeliveryAddressesPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
