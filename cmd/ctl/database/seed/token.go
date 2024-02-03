package seed

import (
	"PowerX/internal/model/crm/trade"
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateTokenExchangeRatios(db *gorm.DB) (err error) {

	var count int64
	if err = db.Model(&trade.TokenExchangeRatio{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init exchange rate  failed"))
	}

	data := DefaultExchangeRecord(db)
	if count == 0 {
		if err = db.Model(&trade.TokenExchangeRatio{}).Create(data).Error; err != nil {
			panic(errors.Wrap(err, "init price book failed"))
		}
	}

	return err
}

func DefaultExchangeRecord(db *gorm.DB) []*trade.TokenExchangeRatio {

	ucDD := powerx.NewDataDictionaryUseCase(db)
	categoryId := ucDD.GetCachedDD(context.Background(), trade.TypeTokenCategory, trade.TokenCategoryPurchase).Id

	return []*trade.TokenExchangeRatio{
		{
			FromCategory: int(categoryId),
			Ratio:        1,
		},
	}
}
