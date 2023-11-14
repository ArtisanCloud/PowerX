package registercode

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetRegisterCodeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetRegisterCodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRegisterCodeLogic {
	return &GetRegisterCodeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetRegisterCodeLogic) GetRegisterCode(req *types.GetRegisterCodeReqeuest) (resp *types.GetRegisterCodeReply, err error) {
	// todo: add your logic here and delete this line

	return
}
