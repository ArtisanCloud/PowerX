package wecom

import (
	"errors"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerSocialite/v2/src/providers"
	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerWeChat/v2/src/work/externalContact"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/config"
	"github.com/gin-gonic/gin"
)

type WeComCustomerService struct {
	Service  *WeComService
	Customer *models.Customer
}

func NewWeComCustomerService(ctx *gin.Context) (r *WeComCustomerService) {
	weComConfig, _ := object.StructToMap(config.AppConfigure.Wechat["wecom"])
	if weComConfig["contact_secret"] != nil {
		weComConfig["secret"] = weComConfig["contact_secret"]
		weComConfig["oauth.scopes"] = []string{"snsapi_base"}
	}
	r = &WeComCustomerService{
		Service:  WeComCustomer,
		Customer: models.NewCustomer(nil),
	}
	return r
}

func SetAuthOpenID(ctx *gin.Context, openid string) {
	ctx.Set("openID", openid)
}

func GetAuthOpenID(ctx *gin.Context) (openid string) {
	value, result := ctx.Get("openID")
	if result {
		openid = value.(string)
	}
	return openid
}

func (srv *WeComCustomerService) GetCustomerByOpenID(db *gorm.DB, openID string) (account *models.Customer, err error) {
	account = &models.Customer{}

	db = db.Where("open_id=?", openID)
	db = db.First(&account)

	return account, db.Error
}

func (srv *WeComCustomerService) GetCustomerByWXExternalUserID(db *gorm.DB, externalUserID string) (account *models.Customer, err error) {

	account = &models.Customer{}

	db = db.Scopes(
		srv.Customer.WhereExternalUserID(externalUserID),
	)

	result := db.First(account)

	return account, result.Error

}

func (srv *WeComCustomerService) GetContactByExternalUserID(ctx *gin.Context, externalUserID string) (user *providers.User, err error) {
	externalClient := WeComCustomer.App.GetComponent("Customer").(*externalContact.Client)
	responseGetUserByID, err := externalClient.Get(externalUserID, "0")
	if responseGetUserByID == nil {
		return nil, errors.New("get wx contract error")
	}

	if responseGetUserByID.ErrCode == 0 {
		user = WeComCustomer.App.OAuth.Provider.Detailed().MapUserToContact(responseGetUserByID)
	} else {
		return nil, errors.New(responseGetUserByID.ErrMSG)
	}
	return user, nil
}
