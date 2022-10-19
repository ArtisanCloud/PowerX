package wx

import (
	configApp "github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
)

type WeComOAuthAPI struct {
	CallbackURL string ` json:"weCom_oauth_callback_url"`
	State       string ` json:"state"`
}

type WeComAPIConfig struct {
	CorpID  string         ` json:"corp_id"`
	AgentID int            ` json:"weCom_agent_id"`
	OAuth   *WeComOAuthAPI ` json:"weCom_oauth_api"`

	AppMessageCallbackURL      string ` json:"app_message_callback_url"`
	CustomerMessageCallbackURL string ` json:"customer_message_callback_url"`
	EmployeeMessageCallbackURL string ` json:"employee_message_callback_url"`
}

type WXAPIConfig struct {
	WeComAPIConfig *WeComAPIConfig ` json:"weCom_api_config"`
}

type WXService struct {
	Context *gin.Context
}

func NewWXService(ctx *gin.Context) (r *WXService) {

	r = &WXService{
		Context: ctx,
	}
	return r
}

func (srv *WXService) GetWXConfig() (config *WXAPIConfig) {

	return &WXAPIConfig{
		WeComAPIConfig: &WeComAPIConfig{
			CorpID:  configApp.G_AppConfigure.WeComConfig.CorpID,
			AgentID: configApp.G_AppConfigure.WeComConfig.WeComAgentID,
			OAuth: &WeComOAuthAPI{
				CallbackURL: configApp.G_AppConfigure.WeComConfig.AppOAuthCallbackURL,
				State:       "code",
			},
		},
	}

}
