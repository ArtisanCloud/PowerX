package wecom

import (
	"errors"
	"fmt"
	fmt2 "github.com/ArtisanCloud/PowerLibs/v2/fmt"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	os2 "github.com/ArtisanCloud/PowerLibs/v2/os"
	"github.com/ArtisanCloud/PowerSocialite/v2/src/providers"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/power"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/kernel/response"
	"github.com/ArtisanCloud/PowerWeChat/v2/src/work"
	requestContactWay "github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/contactWay/request"
	requestMessageTemplate "github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact/messageTemplate/request"
	requestMessage "github.com/ArtisanCloud/PowerWeChat/v2/src/work/message/request"
	requestOAuth "github.com/ArtisanCloud/PowerWeChat/v2/src/work/oauth/request"
	modelPowerWechat "github.com/ArtisanCloud/PowerWeChat/v2/src/work/server/handlers/models"
	"github.com/ArtisanCloud/PowerX/app/models"
	modelWX "github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/ArtisanCloud/PowerX/config/app"
	"github.com/ArtisanCloud/PowerX/config/cache"
	logger "github.com/ArtisanCloud/PowerX/loggerManager"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http"
	"os"
	"reflect"
	"time"
)

type WeComService struct {
	App *work.Work
}

var BufferTextMessage object.HashMap = object.HashMap{}
var timer *time.Timer
var Ticker *time.Ticker = time.NewTicker(time.Second)
var IsBuffing bool

func NewWeComService(ctx *gin.Context) (*WeComService, error) {

	var err error
	wecomCorpID := os.Getenv("corp_id")
	wecomAgentID, err := os2.GetEnvInt("wecom_agent_id")
	wecomSecret := os.Getenv("wecom_secret")
	messageToken := os.Getenv("app_message_token")
	messageAESKey := os.Getenv("app_message_aes_key")
	messageCallbackURL := os.Getenv("app_message_callback_url")
	appOauthCallbackURL := os.Getenv("app_oauth_callback_url")

	if ctx != nil {
		if ctx.GetString("wecomCorpID") != "" {
			wecomCorpID = ctx.GetString("wecomCorpID")
		}
		if ctx.GetInt("wecomAgentID") != 0 {
			wecomAgentID = ctx.GetInt("wecomAgentID")
		}
		if ctx.GetString("wecomSecret") != "" {
			wecomSecret = ctx.GetString("wecomSecret")
		}
		if ctx.GetString("messageToken") != "" {
			messageToken = ctx.GetString("messageToken")
		}
		if ctx.GetString("messageAESKey") != "" {
			messageAESKey = ctx.GetString("messageAESKey")
		}
		if ctx.GetString("messageCallbackURL") != "" {
			messageCallbackURL = ctx.GetString("messageCallbackURL")
		}
		if ctx.GetString("appOauthCallbackURL") != "" {
			appOauthCallbackURL = ctx.GetString("appOauthCallbackURL")
		}
	}

	logger.Logger.Info(fmt.Sprintf("init wecom", zap.Any("corpID", wecomCorpID), zap.Any("wecomAgentID", wecomAgentID)))
	app, err := work.NewWork(&work.UserConfig{
		CorpID:      wecomCorpID,        // 企业微信的corp id，所有企业微信共用一个。
		AgentID:     wecomAgentID,       // 内部应用的app id
		Secret:      wecomSecret,        // 默认内部应用的app secret
		Token:       messageToken,       // 默认内部应用的app token
		AESKey:      messageAESKey,      // 默认内部应用的app aeskey
		CallbackURL: messageCallbackURL, // 内部应用的场景回调设置
		OAuth: work.OAuth{
			Callback: appOauthCallbackURL, // 内部应用的app oauth url
			Scopes:   []string{"snsapi_base"},
		},
		Cache: kernel.NewRedisClient(&kernel.RedisOptions{
			Addr:     cache.G_RedisConfig.Host,
			Password: cache.G_RedisConfig.Password,
			DB:       cache.G_RedisConfig.DB,
		}),
		HttpDebug: true,
	})

	if err != nil {
		return nil, err
	}

	return &WeComService{
		App: app,
	}, nil

}

func (srv *WeComService) SendAddCustomerWelcomeMsg(ctx *gin.Context, contactWay *models.ContactWay, msg *modelPowerWechat.EventExternalUserAdd) (err error) {

	//conclusions := requestContactWay.Conclusions{}
	//err = object.JsonDecode(contactWay.Conclusions, &conclusions)
	//if err != nil {
	//	return err
	//}
	//welcomeMSG, err := srv.ConvertConclusionsToWelcomeMessage(conclusions, msg)

	attachments := []*object.HashMap{}
	err = object.JsonDecode(contactWay.Attachments, attachments)
	if err != nil {
		return err
	}
	welcomeMSG, err := srv.ConvertAttachmentsToWelcomeMessage(attachments, msg)
	if err != nil {
		return err
	}

	result, err := srv.SendWelcomeMsg(ctx, welcomeMSG)
	if result.ErrCode != 0 {
		return errors.New(result.ErrMSG)
	}

	return err
}

func (srv *WeComService) SendDelCustomerWelcomeMsg(ctx *gin.Context, msg *modelPowerWechat.EventExternalUserDelFollowUser) (err error) {

	//contactWay = srv.ServiceContactWay.Get(msg.UserID)

	//_, err = srv.SendWelcomeMsg(ctx, &requestMessageTemplate.RequestSendWelcomeMsg{
	//	msg.WelcomeCode,
	//	//"",
	//	nil,
	//})

	return err
}

func (srv *WeComService) SendMessageToEmployee(ctx *gin.Context, msgType string,
	toUserID string, toParty string, toTag string,
	content interface{}) (err error) {

	if toUserID == "" && toParty == "" && toTag == "" {
		return errors.New("have to offer the sendable object from toUserID,toPart,ToTag")
	}

	weComConfig := app.G_AppConfigure.Wechat["wecom"].(map[string]interface{})
	agentID := weComConfig["agent_id"].(int)

	if msgType == "text" {

		// check if sending messages are in buffer
		_ = srv.bufferTextMessages(ctx,
			agentID, toUserID, toParty, toTag,
			content.(string))

	} else {
		return errors.New("message type is invalid")
	}

	return err
}

func (srv *WeComService) SendWelcomeMsg(ctx *gin.Context, options *requestMessageTemplate.RequestSendWelcomeMsg) (*response.ResponseWork, error) {

	return srv.App.ExternalContactMessageTemplate.SendWelcomeMsg(options)
}

func (srv *WeComService) SendTextToEmployee(ctx *gin.Context, agentID int, toUserID string, toParty string, toTag string, content string) (err error) {

	msg := &requestMessage.RequestMessageSendText{
		requestMessage.RequestMessageSend{
			ToUser:                 toUserID,
			ToParty:                toParty,
			ToTag:                  toTag,
			MsgType:                "text",
			AgentID:                agentID,
			Safe:                   0,
			EnableIDTrans:          0,
			EnableDuplicateCheck:   0,
			DuplicateCheckInterval: 1800,
		},
		&requestMessage.RequestText{
			Content: content,
		},
	}

	mapPara, _ := object.StructToHashMap(msg)
	para, _ := power.HashMapToPower(mapPara)

	response, err := srv.App.Message.Send(para)
	if response == nil || response.ResponseWork == nil {
		err = errors.New("response nil")
	} else if response.ErrCode != 0 {
		err = errors.New(fmt.Sprintf("response error message as :%s", response.ErrMSG))
	}

	return err
}

func (srv *WeComService) bufferTextMessages(ctx *gin.Context,
	agentID int, toUserID string, toParty string, toTag string,
	content string) (message string) {

	key := fmt.Sprintf("%d-%s", agentID, toUserID)

	// init the buffer with key "agentID-toUserID"
	if BufferTextMessage[key] == nil {
		BufferTextMessage[key] = ""
		fmt.Printf("first send key: %s, message:%v \n", key, content)
		srv.SendTextToEmployee(ctx, agentID, toUserID, toParty, toTag, content)
	}

	// if current is not buffering
	if !IsBuffing {
		// setup buffer
		Ticker.Reset(time.Second)
		IsBuffing = true

		go func() {
			i := 0
			for {
				<-Ticker.C
				i++
				fmt.Println("i=", i)
				if i >= 30 {
					// stop ticker after 30s buffer duration
					Ticker.Stop()
					IsBuffing = false

					for _, msg := range BufferTextMessage {
						// send the buffer message with specific key
						if msg != nil {
							strMSG := msg.(string)
							if strMSG != "" {
								fmt.Printf("ticker send key: %s, message:%v \n", key, msg)
								srv.SendTextToEmployee(ctx, agentID, toUserID, toParty, toTag, strMSG)
								BufferTextMessage[key] = nil
							}
						}
					}

					return
				}
			}
		}()

	} else {
		// stack the message into buffer
		bufferString := BufferTextMessage[key].(string)
		BufferTextMessage[key] = fmt.Sprintf("%s\n\n%s", bufferString, content)
	}

	return BufferTextMessage[key].(string)
}

func (srv *WeComService) Authorize(ctx *gin.Context, callbackURI string) {

	// $callbackUrl 为授权回调地址
	callbackUrl := srv.App.GetConfig().GetString("oauth.callback", "") + callbackURI // 需设置可信域名

	// 返回一个 redirect 实例
	redirectURL, _ := srv.App.OAuth.Provider.Redirect(callbackUrl)
	//fmt.Printf("redirect url: %s", redirectURL)
	// 直接跳转到企业微信授权
	http.Redirect(ctx.Writer, ctx.Request, redirectURL, http.StatusFound)

}

func (srv *WeComService) AuthorizeQR(ctx *gin.Context, callbackURI string) {

	// $callbackUrl 为授权回调地址
	callbackUrl := srv.App.GetConfig().GetString("oauth.callback", "") + callbackURI // 需设置可信域名
	srv.App.OAuth.Provider.WithRedirectURL(callbackUrl)

	// 返回一个 redirect 实例
	redirectURL, _ := srv.App.OAuth.Provider.GetQrConnectURL()
	//fmt.Printf("redirect url: %s", redirectURL)
	// 直接跳转到企业微信授权
	http.Redirect(ctx.Writer, ctx.Request, redirectURL, http.StatusFound)

}

func (srv *WeComService) AuthorizedCustomer(ctx *gin.Context) (*providers.User, error) {

	params, _ := ctx.Get("params")
	para := params.(requestOAuth.ParaOAuthCallback)
	cachedToken, err := srv.App.AccessToken.GetToken(false)
	customer, err := srv.App.OAuth.Provider.WithApiAccessToken(cachedToken.AccessToken).ContactFromCode(para.Code)
	if err != nil {
		return nil, err
	}

	return customer, err

}

func (srv *WeComService) AuthorizedEmployee(ctx *gin.Context) (*providers.User, error) {

	params, _ := ctx.Get("params")
	para := params.(requestOAuth.ParaOAuthCallback)
	cachedToken, err := srv.App.AccessToken.GetToken(false)
	user, err := srv.App.OAuth.Provider.WithApiAccessToken(cachedToken.AccessToken).UserFromCode(para.Code)
	if err != nil {
		return nil, err
	}

	// tbf
	//user := src.NewUser(&object.HashMap{
	//	"deviceID": "",
	//	"userID":   "michaelhu",
	//	"errcode":  0,
	//	"errmsg":   "ok",
	//}, service.App.OAuth.Provider)
	//fmt2.Dump(user)

	// 获取用户信息
	//userID := user.GetID()   // 对应企业微信英文名（userid）
	//userRaw := user.GetRaw() // 获取企业微信接口返回的原始信息

	//if userID == "" {
	//	return user, errors.New(fmt.Sprintf("err get user info and raw is : %v", userRaw))
	//}

	return user, err

}

func (srv *WeComService) AuthorizedEmployeeQR(ctx *gin.Context) (*providers.User, error) {

	params, _ := ctx.Get("params")
	para := params.(requestOAuth.ParaOAuthCallbackQRCode)
	cachedToken, err := srv.App.AccessToken.GetToken(false)
	if err != nil {
		return nil, err
	}
	user, err := srv.App.OAuth.Provider.WithApiAccessToken(cachedToken.AccessToken).UserFromCode(para.Code)
	if err != nil {
		return nil, err
	}

	return user, err

}

func (srv *WeComService) IsUserEmployee(user *providers.User) bool {
	return srv.App.OAuth.Provider.IdentifyUserAsEmployee(user) != ""
}

func (srv *WeComService) IsUserCustomer(user *providers.User) bool {
	return srv.App.OAuth.Provider.IdentifyUserAsContact(user) != ""
}

// ------------------------------------------------------------------------------

func (srv *WeComService) ConvertConclusionsToWelcomeMessage(conclusions requestContactWay.Conclusions, msg *modelPowerWechat.EventExternalUserAdd) (welcomeMSG *requestMessageTemplate.RequestSendWelcomeMsg, err error) {

	var text = &requestMessageTemplate.TextOfMessage{}
	attachments := []requestMessageTemplate.MessageTemplateInterface{}
	val := reflect.ValueOf(&conclusions).Elem()
	for i := 0; i < val.NumField(); i++ {
		name := val.Type().Field(i).Name
		value := val.Field(i).Interface()
		fmt2.Dump(name, value)

		switch name {
		case "Image":
			attachment := &requestMessageTemplate.ImageOfMessage{}
			attachment.MsgType = "image"
			attachment.Image = value.(*requestMessageTemplate.Image)
			if attachment.Image.MediaID == "" {
				continue
			}
			attachments = append(attachments, attachment)
			break
		case "Link":
			attachment := &requestMessageTemplate.LinkOfMessage{}
			attachment.MsgType = "link"
			attachment.Link = value.(*requestMessageTemplate.Link)
			if attachment.Link.URL == "" {
				continue
			}
			attachments = append(attachments, attachment)
			break
		case "MiniProgram":
			attachment := &requestMessageTemplate.MiniProgramOfMessage{}
			attachment.MsgType = "miniprogram"
			attachment.MiniProgram = value.(*requestMessageTemplate.MiniProgram)
			if attachment.MiniProgram.AppID == "" {
				continue
			}
			attachments = append(attachments, attachment)
			break
		case "Text":
			text = value.(*requestMessageTemplate.TextOfMessage)

			break
		default:

		}

	}

	welcomeMSG = &requestMessageTemplate.RequestSendWelcomeMsg{
		msg.WelcomeCode,
		text,
		attachments,
	}

	return welcomeMSG, err
}

func (srv *WeComService) ConvertAttachmentsToMessageTemplate(msg *modelWX.WXMessageTemplate) (messageTemplate *requestMessageTemplate.RequestAddMsgTemplate, err error) {

	var text = &requestMessageTemplate.TextOfMessage{}
	attachments := []requestMessageTemplate.MessageTemplateInterface{}

	externalUserIDs := []string{}
	err = object.JsonDecode(msg.ExternalUserIDs, &externalUserIDs)
	if err != nil {
		return nil, err
	}

	arrayAttachments := []*object.HashMap{}
	err = object.JsonDecode(msg.Attachments, &arrayAttachments)
	if err != nil {
		return nil, err
	}

	text = &requestMessageTemplate.TextOfMessage{}
	err = object.JsonDecode(msg.Text, &text)
	if err != nil {
		return nil, err
	}

	_, attachments, err = srv.ConvertAttachmentsToMessageTemplateInterface(arrayAttachments)

	messageTemplate = &requestMessageTemplate.RequestAddMsgTemplate{
		ChatType:       msg.ChatType,
		ExternalUserID: externalUserIDs,
		Sender:         msg.Sender,
		Text:           text,
		Attachments:    attachments,
	}

	return messageTemplate, err
}

func (srv *WeComService) ConvertAttachmentsToWelcomeMessage(arrayAttachments []*object.HashMap, msg *modelPowerWechat.EventExternalUserAdd) (welcomeMSG *requestMessageTemplate.RequestSendWelcomeMsg, err error) {

	var text = &requestMessageTemplate.TextOfMessage{}
	attachments := []requestMessageTemplate.MessageTemplateInterface{}

	text, attachments, err = srv.ConvertAttachmentsToMessageTemplateInterface(arrayAttachments)

	welcomeMSG = &requestMessageTemplate.RequestSendWelcomeMsg{
		msg.WelcomeCode,
		text,
		attachments,
	}

	return welcomeMSG, err
}

func (srv *WeComService) ConvertAttachmentsToMessageTemplateInterface(arrayAttachments []*object.HashMap) (text *requestMessageTemplate.TextOfMessage, attachments []requestMessageTemplate.MessageTemplateInterface, err error) {

	text = &requestMessageTemplate.TextOfMessage{}
	attachments = []requestMessageTemplate.MessageTemplateInterface{}

	for _, attachment := range arrayAttachments {

		if (*attachment)["msgtype"] == nil {
			continue
		}
		msgType := (*attachment)["msgtype"].(string)
		switch msgType {
		case "image":

			messageTemplate := &requestMessageTemplate.ImageOfMessage{}
			err = object.HashMapToStructure(attachment, messageTemplate)
			if err != nil {
				logger.Logger.Error(err.Error())
				continue
			}
			if messageTemplate.Image.MediaID == "" {
				continue
			}
			attachments = append(attachments, messageTemplate)
			break
		case "link":
			messageTemplate := &requestMessageTemplate.LinkOfMessage{}
			err = object.HashMapToStructure(attachment, messageTemplate)
			if err != nil {
				logger.Logger.Error(err.Error())
				continue
			}
			if messageTemplate.Link.URL == "" {
				continue
			}
			attachments = append(attachments, messageTemplate)
			break
		case "miniprogram":
			messageTemplate := &requestMessageTemplate.MiniProgramOfMessage{}
			err = object.HashMapToStructure(attachment, messageTemplate)
			if messageTemplate.MiniProgram.AppID == "" {
				continue
			}
			attachments = append(attachments, messageTemplate)
			break
		case "text":
			err = object.HashMapToStructure(attachment, text)

			break
		default:

		}

	}

	return text, attachments, err
}
