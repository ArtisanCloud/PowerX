package wechat

import (
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

    ls, err := this.wework.ExternalContactGroupChat.List(this.ctx, opt)
    if err != nil {
        logx.Errorf(`wechat.list.customer.group.error: %v`, err)
        return nil, err
    }
    if ls != nil {
        this.gLock.Add(len(ls.GroupChatList))
        for _, chat := range ls.GroupChatList {
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
