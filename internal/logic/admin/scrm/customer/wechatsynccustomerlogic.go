package customer

import (
    "PowerX/internal/svc"
    "PowerX/internal/types"
    "context"

    "github.com/zeromicro/go-zero/core/logx"
)

type WechatSyncCustomerLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewWechatSyncCustomerLogic(ctx context.Context, svcCtx *svc.ServiceContext) *WechatSyncCustomerLogic {
    return &WechatSyncCustomerLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// WechatSyncCustomer
//  @Description:
//  @receiver this
//  @return resp
//  @return err
//
func (this *WechatSyncCustomerLogic) WechatSyncCustomer() (resp *types.WechatSyncCustomerReply, err error) {

    return &types.WechatSyncCustomerReply{
        Status: `success`,
    }, err
}
