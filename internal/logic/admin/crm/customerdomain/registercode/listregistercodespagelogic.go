package registercode

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListRegisterCodesPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListRegisterCodesPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListRegisterCodesPageLogic {
	return &ListRegisterCodesPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListRegisterCodesPageLogic) ListRegisterCodesPage(req *types.ListRegisterCodesPageRequest) (resp *types.ListRegisterCodesPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
