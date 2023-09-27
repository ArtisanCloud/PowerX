package menu

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type QueryMenusLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewQueryMenusLogic(ctx context.Context, svcCtx *svc.ServiceContext) *QueryMenusLogic {
	return &QueryMenusLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *QueryMenusLogic) QueryMenus() (resp *types.QueryMenusReply, err error) {
	// todo: add your logic here and delete this line

	return
}
