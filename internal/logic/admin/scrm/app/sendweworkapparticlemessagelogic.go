package app

import (
	"context"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/request"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SendWeWorkAppArticleMessageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSendWeWorkAppArticleMessageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SendWeWorkAppArticleMessageLogic {
	return &SendWeWorkAppArticleMessageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

//
// SendWeWorkAppArticleMessage
//  @Description:
//  @receiver this
//  @param opt
//  @return resp
//  @return err
//
func (this *SendWeWorkAppArticleMessageLogic) SendWeWorkAppArticleMessage(opt *types.AppMessageArticlesRequest) (resp *types.AppMessageBaseReply, err error) {

	option := this.OPT(opt)

	_, err = this.svcCtx.PowerX.SCRM.Wechat.PushAppWeWorkMessageArticlesRequest(option)

	return &types.AppMessageBaseReply{
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
func (this SendWeWorkAppArticleMessageLogic) OPT(opt *types.AppMessageArticlesRequest) *request.RequestMessageSendNews {

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
