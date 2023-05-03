package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateServiceSpecificLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateServiceSpecificLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateServiceSpecificLogic {
	return &CreateServiceSpecificLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateServiceSpecificLogic) CreateServiceSpecific(req *types.CreateServiceSpecificRequest) (resp *types.CreateServiceSpecificReply, err error) {
	// todo: add your logic here and delete this line

	return
}
