package app

import (
    "context"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/request"

    "PowerX/internal/svc"
    "PowerX/internal/types"

    "github.com/zeromicro/go-zero/core/logx"
)

type WechatAppArticlesLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewWechatAppArticlesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *WechatAppArticlesLogic {
    return &WechatAppArticlesLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// WechatAppArticles
//  @Description: App发送图文信息
//  @receiver this
//  @param opt
//  @return resp
//  @return err
//
func (this *WechatAppArticlesLogic) WechatAppArticles(opt *types.AppMessageArticlesRequest) (resp *types.AppMessageBaseReplay, err error) {

    _, err = this.svcCtx.PowerX.SCRM.Wechat.AppWechatMessageArticles(this.OPT(opt))

    return &types.AppMessageBaseReplay{
        Message: `success`,
    }, err

}

//
// OPT
//  @Description:
//  @receiver this
//  @param opt
//  @return *request.RequestMessageSendNews
//
func (this WechatAppArticlesLogic) OPT(opt *types.AppMessageArticlesRequest) *request.RequestMessageSendNews {

    article := &request.RequestMessageSendNews{RequestMessageSend: request.RequestMessageSend{
        ToUser:  opt.ToUser,
        ToParty: opt.ToParty,
        ToTag:   opt.ToTag,
        MsgType: opt.MsgType,
        AgentID: opt.AgentID},
    }
    arc := &request.RequestNews{}
    for _, val := range opt.News.Article {
        arc.Article = append(arc.Article, &request.RequestNewsArticle{
            Title:       val.Title,
            Description: val.Description,
            URL:         val.URL,
            PicURL:      val.PicURL,
        })
    }
    article.News = arc
    return article
}
