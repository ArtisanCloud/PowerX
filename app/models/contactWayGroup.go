package models

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	database2 "github.com/ArtisanCloud/PowerX/configs/database"
	"gorm.io/gorm"
)

// TableName overrides the table name
func (mdl *ContactWayGroup) TableName() string {
	return mdl.GetTableName(true)
}

type ContactWayGroup struct {
	*database.PowerModel

	ContactWays []*ContactWay `gorm:"foreignKey:GroupUUID;references:UUID" json:"contactWays"`

	Name string `gorm:"unique; column:name" json:"name"`
}

const TABLE_NAME_CONTACT_WAY_GROUP = "contact_way_groups"

func (mdl *ContactWayGroup) GetTableName(needFull bool) string {
	tableName := TABLE_NAME_CONTACT_WAY_GROUP
	if needFull {
		tableName = database2.G_DBConfig.Schemas["option"] + "." + tableName
	}
	return tableName
}

func NewContactWayGroup(mapObject *object.Collection) *ContactWayGroup {
	if mapObject == nil {
		mapObject = object.NewCollection(&object.HashMap{})
	}

	return &ContactWayGroup{
		PowerModel: database.NewPowerModel(),
		Name:       mapObject.GetString("name", ""),
	}
}

/**
 * Association belongings
 */

// -- ContactWays
func (mdl *ContactWayGroup) LoadContactWays(db *gorm.DB, conditions *map[string]interface{}) ([]*ContactWay, error) {
	mdl.ContactWays = []*ContactWay{}
	err := database.AssociationRelationship(db, conditions, mdl, "ContactWays", false).Find(&mdl.ContactWays)
	if err != nil {
		panic(err)
	}
	return mdl.ContactWays, err
}
