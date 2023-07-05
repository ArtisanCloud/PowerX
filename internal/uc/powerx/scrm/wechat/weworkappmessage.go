package wechat

import (
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/request"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/response"
)

//
// PushAppWeWorkMessageArticlesRequest
//  @Description:
//  @receiver this
//  @param opt
//  @return *response.ResponseMessageSend
//  @return error
//
func (this wechatUseCase) PushAppWeWorkMessageArticlesRequest(opt *request.RequestMessageSendNews) (*response.ResponseMessageSend, error) {

    reply, err := this.wework.Message.SendNews(this.ctx, opt)
    if err != nil {
        panic(err)

    } else {
        err = this.help.error(`scrm.push.wework.app.message.articles.error`, reply.ResponseWork)

    }

    return reply, err

}
