package customerdomain

import "PowerX/internal/model"

const SourceFromMP = "mini-program"
const SourceFromOA = "official-account"
const SourceFromWeWork = "wework"

type ExternalId struct {
	OpenIDInMiniProgram           string `gorm:"index"`
	OpenIDInWeChatOfficialAccount string `gorm:"index"`
	OpenIDInWeCom                 string `gorm:"index"`
}

const CustomerUniqueId = "mobile"

type Customer struct {
	//Inviter     *Customer

	model.Model
	Name        string
	Mobile      string `gorm:"unique"`
	Email       string
	InviterID   int64
	Source      string
	Type        string
	IsActivated bool
	ExternalId
}
