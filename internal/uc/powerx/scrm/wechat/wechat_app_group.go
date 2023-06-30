package wechat

import (
    "fmt"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/power"
    kresp "github.com/ArtisanCloud/PowerWeChat/v3/src/kernel/response"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/appChat/request"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/message/appChat/response"
    "github.com/zeromicro/go-zero/core/logx"
)

//
// AppWechatGroupCreate
//  @Description:
//  @receiver this
//  @param option
//  @return reply
//  @return err
//
func (this wechatUseCase) AppWechatGroupCreate(option *request.RequestAppChatCreate) (reply *response.ResponseAppChatCreate, err error) {

    reply, err = this.wework.MessageAppChat.Create(this.ctx, option)
    if err != nil || reply.ErrCode > 0 {
        logx.Error(reply.ErrMSG)
        err = fmt.Errorf(reply.ErrMSG)
    }
    return reply, err

}

//
// AppWechatGroupUpdate
//  @Description:
//  @receiver this
//  @param option
//  @return error
//
func (this wechatUseCase) AppWechatGroupUpdate(option *request.RequestAppChatUpdate) error {

    reply, err := this.wework.MessageAppChat.Update(this.ctx, option)
    if err != nil || reply.ErrCode > 0 {
        logx.Error(reply.ErrMSG)
        err = fmt.Errorf(reply.ErrMSG)
    }
    return err

}

//
// AppWechatGroupDetail
//  @Description:
//  @receiver this
//  @param chatID
//  @return reply
//  @return err
//
func (this wechatUseCase) AppWechatGroupDetail(chatID string) (reply *response.ResponseAppChatGet, err error) {

    reply, err = this.wework.MessageAppChat.Get(this.ctx, chatID)
    if err != nil || reply.ErrCode > 0 {
        logx.Error(reply.ErrMSG)
        err = fmt.Errorf(reply.ErrMSG)
    }
    return reply, err

}

//
// AppWechatGroupMessageArticles
//  @Description:
//  @receiver this
//  @param messages
//  @return *response.ResponseWork
//  @return error
//
func (this *wechatUseCase) AppWechatGroupMessageArticles(messages *power.HashMap) (reply *kresp.ResponseWork, err error) {

    reply, err = this.wework.MessageAppChat.Send(this.ctx, messages)
    if err != nil || reply.ErrCode > 0 {
        logx.Error(reply.ErrMSG)
        err = fmt.Errorf(reply.ErrMSG)
    }
    return reply, err

}
