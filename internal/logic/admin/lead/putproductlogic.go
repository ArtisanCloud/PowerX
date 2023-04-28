package lead

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutProductLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutProductLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutProductLogic {
	return &PutProductLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutProductLogic) PutProduct(req *types.PutLeadRequest) (resp *types.PutLeadReply, err error) {
	// todo: add your logic here and delete this line

	return
}
