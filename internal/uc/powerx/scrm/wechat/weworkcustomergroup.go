package wechat

import (
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/groupChat/request"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/groupChat/response"
)

//
// PullListWeWorkCustomerGroupRequest
//  @Description:
//  @receiver this
//  @param opt
//  @return list
//  @return error
//
func (this *wechatUseCase) PullListWeWorkCustomerGroupRequest(opt *request.RequestGroupChatList) (list []*response.ResponseGroupChatGet, err error) {

    reply, err := this.wework.ExternalContactGroupChat.List(this.ctx, opt)
    if err != nil {
        panic(err)
    } else {
        err = this.help.error(`scrm.wechat.list.customer.group.error`, reply.ResponseWork)
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
