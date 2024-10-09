package datadictionary

import (
	seedCustom "PowerX/cmd/ctl/database/custom/seed"
	seedPro "PowerX/cmd/ctl/database/pro/seed"
	"PowerX/internal/model"
	"PowerX/internal/uc/powerx"
	"PowerX/pkg/slicex"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

var UseCaseDD *powerx.DataDictionaryUseCase

func CreateDataDictionaries(db *gorm.DB) (err error) {

	var count int64
	if err = db.Model(&model.DataDictionaryType{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init data dictionary  failed"))
	}

	UseCaseDD = powerx.NewDataDictionaryUseCase(db)
	data := DefaultDataDictionary()
	proData := seedPro.ProDataDictionary(db)
	customData := seedCustom.CustomDataDictionary(db)
	data = slicex.Concatenate(data, proData, customData)

	if count == 0 {
		if err = db.Model(&model.DataDictionaryType{}).Create(data).Error; err != nil {
			panic(errors.Wrap(err, "init data dictionary failed"))
		}
	}

	return err
}

func DefaultDataDictionary() (data []*model.DataDictionaryType) {

	data = []*model.DataDictionaryType{
		defaultStatusDataDictionary(),
		defaultApprovalStatusDataDictionary(),
		defaultSalesChannelsDataDictionary(),
		defaultPromoteChannelsDataDictionary(),
		defaultSourceDataDictionary(),
		defaultCustomerTypeDataDictionary(),
		defaultProductPlanDataDictionary(),
		defaultProductTypeDataDictionary(),
		defaultArtisanLevelDataDictionary(),
		defaultMediaTypeDataDictionary(),
		defaultOrderTypeDataDictionary(),
		defaultOrderStatusDataDictionary(),
		defaultPaymentTypeDataDictionary(),
		defaultPaymentStatusDataDictionary(),
		defaultTokenCategoryDataDictionary(),
		defaultTokenTransactionDataDictionary(),
		defaultMGMDataDictionary(),
		defaultMembershipTypeDataDictionary(),
		defaultMembershipStatusDataDictionary(),
	}

	return data

}
