package media

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetOAMediaNewsListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetOAMediaNewsListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetOAMediaNewsListLogic {
	return &GetOAMediaNewsListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetOAMediaNewsListLogic) GetOAMediaNewsList() (resp *types.GetOAMediaNewsListReply, err error) {
	// todo: add your logic here and delete this line

	return
}
