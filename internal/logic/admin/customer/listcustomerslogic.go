package customer

import (
	"context"

	"PowerX/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
)

type ListCustomersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListCustomersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListCustomersLogic {
	return &ListCustomersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListCustomersLogic) ListCustomers() error {
	// todo: add your logic here and delete this line

	return nil
}
