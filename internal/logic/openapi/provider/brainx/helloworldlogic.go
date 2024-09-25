package brainx

import (
	"PowerX/internal/provider/brainx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type HelloWorldLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// hello world api for provider demo
func NewHelloWorldLogic(ctx context.Context, svcCtx *svc.ServiceContext) *HelloWorldLogic {
	return &HelloWorldLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *HelloWorldLogic) HelloWorld() (resp *types.HelloWorldResponse, err error) {
	brainXService := brainx.NewBrainXServiceProvider(l.svcCtx)
	message, err := brainXService.HelloWorld(l.ctx)

	return &types.HelloWorldResponse{
		Message: message,
	}, err
}
