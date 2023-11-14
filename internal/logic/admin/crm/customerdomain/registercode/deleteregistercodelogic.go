package registercode

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteRegisterCodeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteRegisterCodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteRegisterCodeLogic {
	return &DeleteRegisterCodeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteRegisterCodeLogic) DeleteRegisterCode(req *types.DeleteRegisterCodeRequest) (resp *types.DeleteRegisterCodeReply, err error) {
	// todo: add your logic here and delete this line

	return
}
