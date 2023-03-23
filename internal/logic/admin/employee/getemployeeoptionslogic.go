package employee

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetEmployeeOptionsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetEmployeeOptionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetEmployeeOptionsLogic {
	return &GetEmployeeOptionsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetEmployeeOptionsLogic) GetEmployeeOptions(req *types.GetEmployeeOptionsRequest) (resp *types.GetEmployeeOptionsReply, err error) {
	// todo: add your logic here and delete this line

	return
}
