package mock

import (
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerSocialite/v2/src/providers"
)

func GetProviderUser() *providers.User {

	user := &providers.User{
		UserInterface: nil,
		Attribute: object.NewAttribute(&object.HashMap{
			"id": "michael",
			"raw": &object.HashMap{
				"DeviceId":        "",
				"OpenId":          "",
				"UserId":          "michael",
				"errcode":         "michael",
				"errmsg":          "ok",
				"external_userid": "",
				"user_ticket":     "",
			},
		}),
	}

	return user
}
