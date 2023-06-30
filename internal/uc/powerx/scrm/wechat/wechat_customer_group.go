package wechat

import (
    "fmt"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/groupChat/request"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/groupChat/response"
    "github.com/zeromicro/go-zero/core/logx"
)

//
// CustomerGroupListWechatWorkRequest
//  @Description:
//  @receiver this
//  @param opt
//  @return list
//  @return error
//
func (this *wechatUseCase) CustomerGroupListWechatWorkRequest(opt *request.RequestGroupChatList) (list []*response.ResponseGroupChatGet, err error) {

    reply, err := this.wework.ExternalContactGroupChat.List(this.ctx, opt)
    if err != nil || reply.ErrCode > 0 {
        logx.Errorf(`scrm.wechat.list.customer.group.error: %v`, err)
        err = fmt.Errorf(reply.ErrMSG)
        return nil, err
    }
    if reply != nil {
        this.gLock.Add(len(reply.GroupChatList))
        for _, chat := range reply.GroupChatList {
            go func(chatID string) {
                get, _ := this.wework.ExternalContactGroupChat.Get(this.ctx, chatID, 1)
                list = append(list, get)
                this.gLock.Done()
            }(chat.ChatID)
        }
        this.gLock.Wait()
    }
    return list, err

}
