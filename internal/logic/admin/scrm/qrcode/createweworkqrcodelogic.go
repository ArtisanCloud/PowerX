package qrcode

import (
    "PowerX/internal/types/errorx"
    "PowerX/pkg/idx"
    "context"
    "fmt"

    "PowerX/internal/svc"
    "PowerX/internal/types"

    "github.com/zeromicro/go-zero/core/logx"
)

type CreateWeWorkQrcodeLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewCreateWeWorkQrcodeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateWeWorkQrcodeLogic {
    return &CreateWeWorkQrcodeLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// CreateWeWorkQrcode
//  @Description: 创建群活码
//  @receiver qrcode
//  @param opt
//  @return resp
//  @return err
//
func (qrcode *CreateWeWorkQrcodeLogic) CreateWeWorkQrcode(opt *types.QrcodeActiveRequest) (resp *types.ActionWeWorkGroupQrcodeActiveReply, err error) {

    if err = qrcode.OPT(opt); err != nil {
        return nil, err
    }

    err = qrcode.svcCtx.PowerX.SCRM.Wechat.CreateWeWorkCustomerGroupQrcodeRequest(opt)
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
func (qrcode *CreateWeWorkQrcodeLogic) OPT(opt *types.QrcodeActiveRequest) (err error) {

    generate, err := idx.Generate()
    if err != nil {
        err = fmt.Errorf(`Qid error`)
    } else {
        opt.Qid = generate
    }

    return err
}
