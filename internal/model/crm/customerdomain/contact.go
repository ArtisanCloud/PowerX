package customerdomain

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
)

type Contact struct {
	powermodel.PowerModel

	Name   string
	Mobile string
	Email  string
	Avatar string
	Status int8
	Active bool
}

func (mdl *Contact) TableName() string {
	return model.PowerXSchema + "." + model.TableNameContact
}

func (mdl *Contact) GetTableName(needFull bool) string {
	tableName := model.TableNameContact
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}
