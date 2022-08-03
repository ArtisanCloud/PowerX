package tests

import (
	"github.com/ArtisanCloud/PowerLibs/v2/fmt"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/models"
	service "github.com/ArtisanCloud/PowerX/app/service"
	"testing"
)

func Test_CreateTokenForCustomer_test(t *testing.T) {

	_, context := MockGin("POST", "/api", nil)

	srvAuth := service.NewAuthService(context)

	customer := models.NewCustomer(object.NewCollection(&object.HashMap{
		"appID":                             "849b613c-03fc-4c6c-96e4-d37e4b6f2d53",
		"openID":                            "749b613c-03fc-4c6c-96e4-d37e4b6f2d53",
		"external_contact.external_user_id": "749b613c-03fc-4c6c-96e4-d37e4b6f2d53",
	}))

	token, result := srvAuth.CreateTokenForCustomer(customer)
	fmt.Dump(token, result)

}
