package app

import (
    "context"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/appChat/request"

    "PowerX/internal/svc"
    "PowerX/internal/types"

    "github.com/zeromicro/go-zero/core/logx"
)

type WechatAppGroupCreateLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewWechatAppGroupCreateLogic(ctx context.Context, svcCtx *svc.ServiceContext) *WechatAppGroupCreateLogic {
    return &WechatAppGroupCreateLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// WechatAppGroupCreate
//  @Description:   App创建群组
//  @receiver this
//  @param opt
//  @return resp
//  @return err
//
func (this *WechatAppGroupCreateLogic) WechatAppGroupCreate(opt *types.AppGroupCreateRequest) (resp *types.AppGroupCreateReplay, err error) {

    reply, err := this.svcCtx.PowerX.SCRM.Wechat.AppWechatGroupCreate(&request.RequestAppChatCreate{
        Name:     opt.Name,
        Owner:    opt.Owner,
        UserList: opt.UserList,
        ChatID:   opt.ChatID,
    })

    return &types.AppGroupCreateReplay{
        ChatID: reply.ChatID,
    }, err
}
