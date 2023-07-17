package qrcode

import (
    "PowerX/internal/types/errorx"
    "context"
    "fmt"

    "PowerX/internal/svc"
    "PowerX/internal/types"

    "github.com/zeromicro/go-zero/core/logx"
)

type UpdateWeWorkQrcodeLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewUpdateWeWorkQrcodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateWeWorkQrcodeLogic {
    return &UpdateWeWorkQrcodeLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// UpdateWeWorkQrcode
//  @Description: 更新群活码
//  @receiver qrcode
//  @param opt
//  @return resp
//  @return err
//
func (qrcode *UpdateWeWorkQrcodeLogic) UpdateWeWorkQrcode(opt *types.QrcodeActiveRequest) (resp *types.ActionWeWorkGroupQrcodeActiveReply, err error) {

    if err = qrcode.OPT(opt); err != nil {
        return nil, err
    }

    err = qrcode.svcCtx.PowerX.SCRM.Wechat.UpdateWeWorkCustomerGroupQrcodeRequest(opt)
    if err != nil {
        return nil, errorx.ErrCreateObject
    }

    return &types.ActionWeWorkGroupQrcodeActiveReply{
        Status: `success`,
    }, err
}

//
// OPT
//  @Description:
//  @receiver qrcode
//  @param opt
//  @return err
//
func (qrcode *UpdateWeWorkQrcodeLogic) OPT(opt *types.QrcodeActiveRequest) (err error) {

    if opt.Name == `` {
        err = fmt.Errorf(`Name error`)
    } else if opt.SceneLink == `` {
        err = fmt.Errorf(`SceneLink error`)
    } else if opt.RealQrcodeLink == `` {
        err = fmt.Errorf(`RealQrcode error`)
    } else if opt.ExpiryDate == 0 {
        err = fmt.Errorf(`ExpiryDate error`)
    } else if opt.Owner == nil {
        err = fmt.Errorf(`Owner error`)
    } else if opt.Qid == `` {
        err = fmt.Errorf(`Qid error`)
    }

    return err
}
