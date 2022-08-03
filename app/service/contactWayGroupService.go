package service

import (
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ContactWayGroupService struct {
	Service         *Service
	ContactWayGroup *models.ContactWayGroup
}

/**
 ** 初始化构造函数
 */
func NewContactWayGroupService(ctx *gin.Context) (r *ContactWayGroupService) {
	r = &ContactWayGroupService{
		Service:         NewService(ctx),
		ContactWayGroup: models.NewContactWayGroup(nil),
	}
	return r
}

func (srv *ContactWayGroupService) GetList(db *gorm.DB, conditions *map[string]interface{}) (arrayContactWayGroups []*models.ContactWayGroup, err error) {

	arrayContactWayGroups = []*models.ContactWayGroup{}

	err = database.GetAllList(db, conditions, &arrayContactWayGroups, nil)

	return arrayContactWayGroups, err
}

func (srv *ContactWayGroupService) UpsertContactWayGroups(db *gorm.DB, uniqueName string, contactWayGroups []*models.ContactWayGroup) error {

	if len(contactWayGroups) <= 0 {
		return nil
	}

	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: uniqueName}},
		DoUpdates: clause.AssignmentColumns(database.GetModelFields(models.ContactWayGroup{})),
	}).Create(&contactWayGroups)

	return result.Error
}

func (srv *ContactWayGroupService) UpsertContactWayGroup(db *gorm.DB, contactWayGroup *models.ContactWayGroup, withAssociation bool) (savedContactWayGroup *models.ContactWayGroup, err error) {

	//contactWayGroup.UpdatedAt = time.Now()
	if contactWayGroup.UUID == "" {
		contactWayGroup.UUID = uuid.NewString()
		//contactWayGroup.CreatedAt = time.Now()
		savedContactWayGroup, err = srv.SaveContactWayGroup(db, contactWayGroup)
	} else {
		savedContactWayGroup, err = srv.UpdateContactWayGroup(db, contactWayGroup, withAssociation)
	}

	return savedContactWayGroup, err
}

func (srv *ContactWayGroupService) SaveContactWayGroup(db *gorm.DB, contactWayGroup *models.ContactWayGroup) (*models.ContactWayGroup, error) {

	db = db.Create(contactWayGroup)

	return contactWayGroup, db.Error
}

func (srv *ContactWayGroupService) UpdateContactWayGroup(db *gorm.DB, contactWayGroup *models.ContactWayGroup, withAssociation bool) (*models.ContactWayGroup, error) {

	if !withAssociation {
		db = db.Omit(clause.Associations)
	}
	db = db.
		//Debug().
		Updates(contactWayGroup)

	return contactWayGroup, db.Error
}

func (srv *ContactWayGroupService) DeleteContactWayGroups(db *gorm.DB, uuids []string) error {
	// delete contactWay
	db = db.
		//Debug().
		Where("uuid in (?)", uuids).
		Delete(&models.ContactWayGroup{})

	return db.Error
}

func (srv *ContactWayGroupService) DeleteContactWayGroup(db *gorm.DB, contactWayGroup *models.ContactWayGroup) error {
	// delete contactWay
	db = db.Delete(contactWayGroup)

	return db.Error
}

func (srv *ContactWayGroupService) GetContactWayGroups(db *gorm.DB, uuids []string) (contactWayGroups []*models.ContactWayGroup, err error) {

	contactWayGroups = []*models.ContactWayGroup{}

	db = db.Where("uuid in (?)", uuids)
	result := db.Find(&contactWayGroups)
	return contactWayGroups, result.Error
}

func (srv *ContactWayGroupService) GetContactWayGroup(db *gorm.DB, uuid string) (contactWayGroup *models.ContactWayGroup, err error) {

	contactWayGroup = &models.ContactWayGroup{}

	db = db.Scopes(
		database.WhereUUID(uuid),
	)
	result := db.First(contactWayGroup)
	return contactWayGroup, result.Error
}

func (srv *ContactWayGroupService) GetContactWayGroupsByOpenIDs(db *gorm.DB, openids []string) (contactWayGroups []*models.ContactWayGroup, err error) {

	contactWayGroups = []*models.ContactWayGroup{}

	db = db.Where("open_id in (?)", openids)
	result := db.Find(&contactWayGroups)
	return contactWayGroups, result.Error
}

func (srv *ContactWayGroupService) GetContactWayGroupByOpenID(db *gorm.DB, openID string) (contactWayGroup *models.ContactWayGroup, err error) {

	contactWayGroup = &models.ContactWayGroup{}

	db = db.Where("open_id=(?)", openID)
	result := db.First(contactWayGroup)
	return contactWayGroup, result.Error
}
