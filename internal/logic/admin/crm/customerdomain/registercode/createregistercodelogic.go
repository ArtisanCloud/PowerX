package registercode

import (
	"PowerX/internal/model/crm/customerdomain"
	"context"
	"github.com/golang-module/carbon/v2"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateRegisterCodeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateRegisterCodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateRegisterCodeLogic {
	return &CreateRegisterCodeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateRegisterCodeLogic) CreateRegisterCode(req *types.CreateRegisterCodeRequest) (resp *types.CreateRegisterCodeReply, err error) {

	code := TransformRequestToRegisterCode(req)

	err = l.svcCtx.PowerX.RegisterCode.CreateRegisterCode(l.ctx, code)

	return &types.CreateRegisterCodeReply{
		code.Id,
	}, err

}

func TransformRequestToRegisterCode(req *types.CreateRegisterCodeRequest) *customerdomain.RegisterCode {
	expiredAt := carbon.Parse(req.ExpiredAt).ToStdTime()
	mdlRegisterCode := &customerdomain.RegisterCode{
		Code:               req.Code,
		RegisterCustomerID: req.RegisterCustomerID,
		ExpiredAt:          expiredAt,
	}
	mdlRegisterCode.Id = req.Id
	return mdlRegisterCode
}
