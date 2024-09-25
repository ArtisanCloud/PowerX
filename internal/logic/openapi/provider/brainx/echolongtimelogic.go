package brainx

import (
	"PowerX/internal/provider/brainx"
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

type EchoLongTimeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// timeout api for provider demo
func NewEchoLongTimeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *EchoLongTimeLogic {
	return &EchoLongTimeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *EchoLongTimeLogic) EchoLongTime(req *types.EchoLongTimeRequest) (resp *types.EchoLongTimeResponse, err error) {

	//time.Sleep(60 * time.Second)
	//return nil, nil

	brainXService := brainx.NewBrainXServiceProvider(l.svcCtx)
	message, err := brainXService.EchoLongTime(l.ctx, req.Timeout)

	return &types.EchoLongTimeResponse{
		Message: message,
	}, err
}
