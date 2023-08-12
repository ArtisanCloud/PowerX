package token

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateTokenProductLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateTokenProductLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateTokenProductLogic {
	return &CreateTokenProductLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateTokenProductLogic) CreateTokenProduct(req *types.CreateProductRequest) (resp *types.CreateProductReply, err error) {
	// todo: add your logic here and delete this line

	return
}
