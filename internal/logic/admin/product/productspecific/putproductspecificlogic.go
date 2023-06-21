package productspecific

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutProductSpecificLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutProductSpecificLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutProductSpecificLogic {
	return &PutProductSpecificLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutProductSpecificLogic) PutProductSpecific(req *types.PutProductSpecificRequest) (resp *types.PutProductSpecificReply, err error) {
	// todo: add your logic here and delete this line

	return
}
