package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutServiceSpecificLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutServiceSpecificLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutServiceSpecificLogic {
	return &PutServiceSpecificLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutServiceSpecificLogic) PutServiceSpecific(req *types.PutServiceSpecificRequest) (resp *types.PutServiceSpecificReply, err error) {
	// todo: add your logic here and delete this line

	return
}
