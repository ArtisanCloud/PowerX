package qrcode

import (
    "PowerX/internal/types/errorx"
    "context"

    "PowerX/internal/svc"
    "PowerX/internal/types"

    "github.com/zeromicro/go-zero/core/logx"
)

type DisableWeWorkQrcodeLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewDisableWeWorkQrcodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DisableWeWorkQrcodeLogic {
    return &DisableWeWorkQrcodeLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// DisableWeWorkQrcode
//  @Description: 禁用群活码
//  @receiver l
//  @param req
//  @return resp
//  @return err
//
func (qrcode *DisableWeWorkQrcodeLogic) DisableWeWorkQrcode(opt *types.ActionRequest) (resp *types.ActionWeWorkGroupQrcodeActiveReply, err error) {

    if opt.Qid == `` {
        return nil, errorx.ErrBadRequest
    }

    err = qrcode.svcCtx.PowerX.SCRM.Wechat.ActionCustomerGroupQrcode(opt.Qid, 2)
    if err != nil {
        return nil, errorx.ErrDeleteObject
    }

    return &types.ActionWeWorkGroupQrcodeActiveReply{
        Status: `success`,
    }, err

}
