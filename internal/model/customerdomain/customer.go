package customerdomain

import (
	"PowerX/internal/model/powermodel"
)

type ExternalId struct {
	OpenIDInMiniProgram           string `gorm:"index"`
	OpenIDInWeChatOfficialAccount string `gorm:"index"`
	OpenIDInWeCom                 string `gorm:"index"`
}

const CustomerUniqueId = "mobile"

type Customer struct {
	//Inviter     *Customer

	powermodel.PowerModel
	Name        string
	Mobile      string `gorm:"unique"`
	Email       string
	InviterID   int64
	Source      int
	Type        string
	IsActivated bool
	ExternalId
}
