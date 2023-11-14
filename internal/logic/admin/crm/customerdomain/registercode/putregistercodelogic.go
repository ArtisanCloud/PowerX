package registercode

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutRegisterCodeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutRegisterCodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutRegisterCodeLogic {
	return &PutRegisterCodeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutRegisterCodeLogic) PutRegisterCode(req *types.PutRegisterCodeRequest) (resp *types.PutRegisterCodeReply, err error) {
	// todo: add your logic here and delete this line

	return
}
