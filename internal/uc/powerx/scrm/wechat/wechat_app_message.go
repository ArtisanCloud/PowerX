package wechat

import (
    "fmt"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/request"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/response"
    "github.com/zeromicro/go-zero/core/logx"
)

//
// AppWechatMessageArticles
//  @Description:
//  @receiver this
//  @param opt
//  @return *response.ResponseMessageSend
//  @return error
//
func (this wechatUseCase) AppWechatMessageArticles(opt *request.RequestMessageSendNews) (*response.ResponseMessageSend, error) {

    reply, err := this.wework.Message.SendNews(this.ctx, opt)
    if err != nil || reply.ErrCode > 0 {
        logx.Errorf(`scrm.wechat.app.message.articles.error. %v`, reply.ErrMSG)
        err = fmt.Errorf(reply.ErrMSG)
    }

    return reply, err

}
