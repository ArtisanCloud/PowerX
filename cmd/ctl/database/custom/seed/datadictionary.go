package seed

import (
	"PowerX/internal/model"
	"PowerX/internal/uc/powerx"
	"gorm.io/gorm"
)

var UseCaseDD *powerx.DataDictionaryUseCase

func CustomDataDictionary(db *gorm.DB) (data []*model.DataDictionaryType) {

	UseCaseDD = powerx.NewDataDictionaryUseCase(db)

	return data

}
