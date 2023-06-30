package app

import (
    "PowerX/internal/svc"
    "PowerX/internal/types"
    "PowerX/internal/uc/powerx/scrm/wechat"
    "context"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/power"

    "github.com/zeromicro/go-zero/core/logx"
)

type WechatAppGroupMessageArticlesLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewWechatAppGroupMessageArticlesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *WechatAppGroupMessageArticlesLogic {
    return &WechatAppGroupMessageArticlesLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// WechatAppGroupMessageArticles
//  @Description:   App发送群组图文信息
//  @receiver this
//  @param opt
//  @return resp
//  @return err
//
func (this *WechatAppGroupMessageArticlesLogic) WechatAppGroupMessageArticles(opt *types.AppGroupMessageArticleRequest) (resp *types.AppGroupMessageReplay, err error) {

    dto, err := this.DTO(opt)
    if err != nil {
        logx.Error(err)
        return nil, err
    }
    _, err = this.svcCtx.PowerX.SCRM.Wechat.AppWechatGroupMessageArticles(dto)

    return &types.AppGroupMessageReplay{
        ChatID: opt.ChatID,
    }, err

}

//
// DTO
//  @Description:
//  @receiver this
//  @param opt
//  @return *power.HashMap
//  @return error
//
func (this *WechatAppGroupMessageArticlesLogic) DTO(opt *types.AppGroupMessageArticleRequest) (*power.HashMap, error) {
    option := wechat.WechatAppRequestBase{
        ChatID:  opt.ChatID,
        MsgType: `news`,
        Safe:    0,
        News:    wechat.WechatAppRequestNews{},
    }
    arc := wechat.WechatAppRequestNews{}
    arc.Article = append(arc.Article, &wechat.WechatAppRequestNewsArticle{
        Title:       opt.Title,
        Description: opt.Description,
        URL:         opt.URL,
        PicURL:      opt.PicURL,
    })
    option.News = arc
    hashMap, err := power.StructToHashMap(option)

    return hashMap,
        err
}
