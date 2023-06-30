package app

import (
    "context"

    "PowerX/internal/svc"
    "PowerX/internal/types"

    "github.com/zeromicro/go-zero/core/logx"
)

type WechatAppGroupDetailLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewWechatAppGroupDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *WechatAppGroupDetailLogic {
    return &WechatAppGroupDetailLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// WechatAppGroupDetail
//  @Description:	App群组详情
//  @receiver this
//  @param req
//  @return resp
//  @return err
//
func (this *WechatAppGroupDetailLogic) WechatAppGroupDetail(req *types.AppGroupListRequest) (resp *types.AppGroupDetailReplay, err error) {

    reply, err := this.svcCtx.PowerX.SCRM.Wechat.AppWechatGroupDetail(req.ChatID)
    if err != nil || reply.ChatInfo == nil {
        return nil, err
    }

    return &types.AppGroupDetailReplay{
        reply.ChatInfo,
    }, err
}
