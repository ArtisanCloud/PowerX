package qrcode

import (
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteWeWorkQrcodeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteWeWorkQrcodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteWeWorkQrcodeLogic {
	return &DeleteWeWorkQrcodeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

//
// DeleteWeWorkQrcode
//  @Description: 删除群活码
//  @receiver l
//  @param req
//  @return resp
//  @return err
//
func (qrcode *DeleteWeWorkQrcodeLogic) DeleteWeWorkQrcode(opt *types.ActionRequest) (resp *types.ActionWeWorkGroupQrcodeActiveReply, err error) {

	if opt.Qid == `` {
		return nil, errorx.ErrBadRequest
	}

	err = qrcode.svcCtx.PowerX.SCRM.Wechat.ActionCustomerGroupQrcode(opt.Qid, 3)
	if err != nil {
		return nil, errorx.ErrDeleteObject
	}

	return &types.ActionWeWorkGroupQrcodeActiveReply{
		Status: `success`,
	}, err
}
