package logic

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetHomeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetHomeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetHomeLogic {
	return &GetHomeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetHomeLogic) GetHome() (resp *types.GetHomeReply, err error) {
	return &types.GetHomeReply{
		Greet:       "Hello, I am PowerX!",
		Description: "This is awesome! you create me and make me alive",
		Version:     "V1.0.1",
	}, nil
}
