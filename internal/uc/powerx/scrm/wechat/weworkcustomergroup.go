package wechat

import (
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/groupChat/request"
	"github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/groupChat/response"
	creq "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/messageTemplate/request"
	crsp "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/messageTemplate/response"
	"time"
)

// PullListWeWorkCustomerGroupRequest
//
//	@Description:
//	@receiver this
//	@param opt
//	@return list
//	@return error
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
				if get.ErrCode == 0 {
					list = append(list, get)
				}
				this.gLock.Done()
			}(chat.ChatID)
		}
		this.gLock.Wait()
	}
	return list, err

}

// PushWoWorkCustomerTemplateRequest
//
//	@Description:
//	@receiver this
//	@param opt
//	@return *crsp.ResponseAddMessageTemplate
//	@return error
func (this *wechatUseCase) PushWoWorkCustomerTemplateRequest(opt *creq.RequestAddMsgTemplate, sendTime int64) (*crsp.ResponseAddMessageTemplate, error) {

	if sendTime > time.Now().Unix() {

		this.pushTimerMessageToKV(AppGroupCustomerMessageTimerTypeByte, sendTime, opt)

	}
	reply, err := this.wework.ExternalContactMessageTemplate.AddMsgTemplate(this.ctx, opt)
	if err != nil {
		panic(err)
	} else {
		err = this.help.error(`scrm.push.wowork.customer.message.error.`, reply.ResponseWork)
	}
	return reply, err

}
