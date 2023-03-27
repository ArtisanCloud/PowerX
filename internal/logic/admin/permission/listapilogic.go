package permission

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListAPILogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListAPILogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAPILogic {
	return &ListAPILogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListAPILogic) ListAPI(req *types.ListAPIRequest) (resp *types.ListAPIReply, err error) {
	// todo: add your logic here and delete this line

	return
}
