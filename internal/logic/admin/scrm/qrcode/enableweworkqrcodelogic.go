package qrcode

import (
    "PowerX/internal/types/errorx"
    "context"

    "PowerX/internal/svc"
    "PowerX/internal/types"

    "github.com/zeromicro/go-zero/core/logx"
)

type EnableWeWorkQrcodeLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewEnableWeWorkQrcodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *EnableWeWorkQrcodeLogic {
    return &EnableWeWorkQrcodeLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// EnableWeWorkQrcode
//  @Description: 启用群活码
//  @receiver l
//  @param req
//  @return resp
//  @return err
//
func (qrcode *EnableWeWorkQrcodeLogic) EnableWeWorkQrcode(opt *types.ActionRequest) (resp *types.ActionWeWorkGroupQrcodeActiveReply, err error) {

    if opt.Qid == `` {
        return nil, errorx.ErrBadRequest
    }

    err = qrcode.svcCtx.PowerX.SCRM.Wechat.ActionCustomerGroupQrcode(opt.Qid, 1)
    if err != nil {
        return nil, errorx.ErrDeleteObject
    }

    return &types.ActionWeWorkGroupQrcodeActiveReply{
        Status: `success`,
    }, err
}
