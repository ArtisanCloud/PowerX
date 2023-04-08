package customer

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListWeWorkCustomersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListWeWorkCustomersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListWeWorkCustomersLogic {
	return &ListWeWorkCustomersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListWeWorkCustomersLogic) ListWeWorkCustomers(req *types.ListWeWorkCustomersRequest) (resp *types.ListWeWorkCustomersReply, err error) {
	// todo: add your logic here and delete this line

	return
}
