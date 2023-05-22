package billing

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListBillingAddressesPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListBillingAddressesPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListBillingAddressesPageLogic {
	return &ListBillingAddressesPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListBillingAddressesPageLogic) ListBillingAddressesPage(req *types.ListBillingAddressesPageRequest) (resp *types.ListBillingAddressesPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
