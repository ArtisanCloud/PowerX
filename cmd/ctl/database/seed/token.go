package seed

import (
	"PowerX/internal/model/trade"
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func CreateTokenExchangeRatios(db *gorm.DB) (err error) {

	var count int64
	if err = db.Model(&trade.ExchangeRatio{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init exchange rate  failed"))
	}

	data := DefaultExchangeRecord(db)
	if count == 0 {
		if err = db.Model(&trade.ExchangeRatio{}).Create(data).Error; err != nil {
			panic(errors.Wrap(err, "init price book failed"))
		}
	}

	return err
}

func DefaultExchangeRecord(db *gorm.DB) []*trade.ExchangeRatio {

	ucDD := powerx.NewDataDictionaryUseCase(db)
	categoryId := ucDD.GetCachedDD(context.Background(), trade.TypeTokenCategory, trade.TokenCategoryPurchase).Id

	return []*trade.ExchangeRatio{
		{
			FromCategory: int(categoryId),
			Ratio:        1,
		},
	}

}
