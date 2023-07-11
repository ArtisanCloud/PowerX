package customer

import (
    "PowerX/internal/svc"
    "PowerX/internal/types"
    "context"
    "github.com/ArtisanCloud/PowerWeChat/v3/src/work/externalContact/messageTemplate/request"
    "github.com/zeromicro/go-zero/core/logx"
)

type SendWeWorkCustomerGroupMessageLogic struct {
    logx.Logger
    ctx    context.Context
    svcCtx *svc.ServiceContext
}

func NewSendWeWorkCustomerGroupMessageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SendWeWorkCustomerGroupMessageLogic {
    return &SendWeWorkCustomerGroupMessageLogic{
        Logger: logx.WithContext(ctx),
        ctx:    ctx,
        svcCtx: svcCtx,
    }
}

//
// SendWeWorkCustomerGroupMessage
//  @Description:
//  @receiver message
//  @param opt
//  @return resp
//  @return err
//
func (message *SendWeWorkCustomerGroupMessageLogic) SendWeWorkCustomerGroupMessage(opt *types.WeWorkAddMsgTemplateRequest) (resp *types.WeWorkAddMsgTemplateResponse, err error) {

    template, err := message.svcCtx.PowerX.SCRM.Wechat.PushWoWorkCustomerTemplateRequest(message.OPT(opt))

    return &types.WeWorkAddMsgTemplateResponse{
        FailList: template.FailList,
        MsgId:    template.MsgID,
    }, err

}

//
// OPT
//  @Description:
//  @receiver message
//  @param opt
//  @return *request.RequestAddMsgTemplate
//
func (message *SendWeWorkCustomerGroupMessageLogic) OPT(opt *types.WeWorkAddMsgTemplateRequest) *request.RequestAddMsgTemplate {
    option := &request.RequestAddMsgTemplate{
        ChatType:       opt.ChatType,
        ExternalUserID: opt.ExternalUserId,
        Sender:         opt.Sender,
        Text:           message.text(opt.Text),
        Attachments:    message.attachments(opt.Attachments),
    }
    return option
}

//
// text
//  @Description:
//  @receiver message
//  @param msg
//  @return *request.TextOfMessage
//
func (message *SendWeWorkCustomerGroupMessageLogic) text(msg *types.WeWorkTextOfMessage) *request.TextOfMessage {

    if msg != nil {
        return &request.TextOfMessage{msg.Content}
    }
    return nil
}

//
// attachments
//  @Description:
//  @receiver message
//  @param contents
//  @return attachments
//
func (message *SendWeWorkCustomerGroupMessageLogic) attachments(contents []types.Content) (attachmentsMessageTemplateInterface []request.MessageTemplateInterface) {

    if len(contents) > 0 {
        attr := new(attachment)
        for _, content := range contents {
            attr.MsgType = content.Link.MsgType
            attr.Link = message.attachmentLink(&content.Link)
            attachmentsMessageTemplateInterface = append(attachmentsMessageTemplateInterface, attr)
        }
    }
    return attachmentsMessageTemplateInterface
}

//
//
//  @Description:
//  @receiver message
//  @param image
//  @return *request.Image
//
func (message *SendWeWorkCustomerGroupMessageLogic) attachmentImage(image *types.Image) *request.Image {

    if image != nil {
        return &request.Image{
            MediaID: image.MediaId,
            PicURL:  image.PicUrl,
        }
    }
    return nil

}

//
// attachmentLink
//  @Description:
//  @receiver message
//  @param link
//  @return *request.Link
//
func (message *SendWeWorkCustomerGroupMessageLogic) attachmentLink(link *types.Link) *request.Link {

    if link != nil {
        return &request.Link{
            Title:  link.Title,
            PicURL: link.PicURL,
            Desc:   link.Desc,
            URL:    link.URL,
        }
    }
    return nil

}

type attachment request.Attachment

func (attachment *attachment) GetMsgType() string {

    return ``
}
