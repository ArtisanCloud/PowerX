package payment

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListPaymentsPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListPaymentsPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListPaymentsPageLogic {
	return &ListPaymentsPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListPaymentsPageLogic) ListPaymentsPage(req *types.ListPaymentsPageRequest) (resp *types.ListPaymentsPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
