package employee

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"context"
	"github.com/zeromicro/go-zero/core/logx"
)

type CreateEmployeeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateEmployeeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateEmployeeLogic {
	return &CreateEmployeeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateEmployeeLogic) CreateEmployee(req *types.CreateEmployeeRequest) (resp *types.CreateEmployeeReply, err error) {

}
