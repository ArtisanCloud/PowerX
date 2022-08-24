package service

import (
	"errors"
	"github.com/ArtisanCloud/PowerLibs/v2/database"
	"github.com/ArtisanCloud/PowerLibs/v2/database/tag"
	object2 "github.com/ArtisanCloud/PowerLibs/v2/object"
	modelWX "github.com/ArtisanCloud/PowerX/app/models/wx"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TagService struct {
	*Service
	TagPivot *tag.RTagToObject
	TagGroup *tag.TagGroup
	Tag      *tag.Tag
}

func NewTagService(ctx *gin.Context) (r *TagService) {
	r = &TagService{
		Service:  NewService(ctx),
		TagPivot: &tag.RTagToObject{},
		TagGroup: tag.NewTagGroup(nil),
		Tag:      tag.NewTag(nil),
	}
	return r
}

func (srv *TagService) GetGroupList(db *gorm.DB, conditions *map[string]interface{}, page int, pageSize int) (pagination *database.Pagination, err error) {

	arrayTags := []*tag.TagGroup{}
	preloads := []string{"Tags"}

	pagination, err = database.GetList(db, conditions, &arrayTags, preloads, page, pageSize)

	return pagination, err
}

func (srv *TagService) QueryTagList(db *gorm.DB, tagType *int, groupID *string) (arrayTags []*tag.Tag, err error) {

	arrayTags = []*tag.Tag{}

	conditions := &map[string]interface{}{}
	if *tagType > 0 {
		(*conditions)["type"] = *tagType
	}
	if *groupID != "" {
		(*conditions)["group_id"] = *groupID
	}

	err = database.GetAllList(db, conditions, &arrayTags, nil)

	return arrayTags, err
}

func (srv *TagService) CreateTagGroupWithTags(db *gorm.DB, group *tag.TagGroup, tags []*tag.Tag) (err error) {

	if len(tags) <= 0 {
		return errors.New("tags is empty")
	}

	err = db.Transaction(func(tx *gorm.DB) error {

		err = srv.UpsertTagGroups(tx, []*tag.TagGroup{group}, nil)
		if err != nil {
			return err
		}

		err = srv.UpsertTags(tx, tags, nil)
		if err != nil {
			return err
		}
		return err
	})

	return err
}

func (srv *TagService) CheckTagGroupNameAvailable(db *gorm.DB, group *tag.TagGroup) (err error) {

	result := db.
		//Debug().
		Where("group_name", group.GroupName).
		Where("owner_type", group.OwnerType).
		Where("index_tag_group_id != ?", group.UniqueID).
		First(&tag.TagGroup{})

	if result.Error != nil && errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil
	}

	if result.Error != nil {
		return result.Error
	}

	err = errors.New("tag group name is not available")

	return err
}

func (srv *TagService) UpdateTagGroupWithTags(db *gorm.DB, group *tag.TagGroup, tags []*tag.Tag) (err error) {

	if len(tags) <= 0 {
		return errors.New("tags is empty")
	}

	err = db.Transaction(func(tx *gorm.DB) error {

		err = srv.CheckTagGroupNameAvailable(db, group)
		if err != nil {
			return err
		}

		err = srv.UpsertTagGroups(tx, []*tag.TagGroup{group}, nil)
		if err != nil {
			return err
		}

		err = srv.DeleteTagsByGroupIDs(tx, []string{group.UniqueID})
		if err != nil {
			return err
		}
		err = srv.UpsertTags(tx, tags, nil)
		if err != nil {
			return err
		}
		return err
	})

	return err
}

func (srv *TagService) UpsertTagGroups(db *gorm.DB, tagGroups []*tag.TagGroup, fieldsToUpdate []string) error {

	return database.UpsertModelsOnUniqueID(db, &tag.TagGroup{}, tag.TAG_GROUP_UNIQUE_ID, tagGroups, fieldsToUpdate)
}

func (srv *TagService) UpsertTags(db *gorm.DB, tags []*tag.Tag, fieldsToUpdate []string) error {

	return database.UpsertModelsOnUniqueID(db, &tag.Tag{}, tag.TAG_UNIQUE_ID, tags, fieldsToUpdate)
}

func (srv *TagService) DeleteTagGroupsWithTags(db *gorm.DB, groupUniqueIDs []string, tagUniqueIDs []string) (err error) {

	err = db.Transaction(func(tx *gorm.DB) error {

		if len(groupUniqueIDs) > 0 {
			err = srv.DeleteTagsByGroupIDs(tx, groupUniqueIDs)
			if err != nil {
				return err
			}
			err = srv.DeleteTagGroupsByIDs(tx, groupUniqueIDs)
			if err != nil {
				return err
			}
		}

		if len(tagUniqueIDs) > 0 {
			err = srv.DeleteTagsByIDs(tx, tagUniqueIDs)
			if err != nil {
				return err
			}
		}

		return err
	})

	return err
}

func (srv *TagService) DeleteTagGroupsByIDs(db *gorm.DB, groupIDs []string) error {

	db = db.
		//Debug().
		Where("index_tag_group_id in (?)", groupIDs).
		Delete(&tag.TagGroup{})

	return db.Error
}

func (srv *TagService) DeleteTagsByGroupIDs(db *gorm.DB, groupIDs []string) error {

	db = db.
		//Debug().
		Where("group_id in (?)", groupIDs).
		Delete(&tag.Tag{})

	return db.Error
}

func (srv *TagService) DeleteTagsByIDs(db *gorm.DB, tagIDs []string) error {
	db = db.
		//Debug().
		Where("index_tag_id in (?)", tagIDs).
		Delete(&tag.Tag{})

	return db.Error
}

func (srv *TagService) DeleteTagByID(db *gorm.DB, tagID string) error {
	db = db.
		//Debug().
		Where("index_tag_id", tagID).
		Delete(&tag.Tag{})

	return db.Error
}

func (srv *TagService) GetTagGroupByID(db *gorm.DB, tagGroupID string) (group *tag.TagGroup, err error) {

	group = &tag.TagGroup{}

	preloads := []string{"Tags"}

	condition := &map[string]interface{}{
		"index_tag_group_id": tagGroupID,
	}
	err = database.GetFirst(db, condition, group, preloads)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return group, err

}

func (srv *TagService) GetTagGroupsByIDs(db *gorm.DB, arrayGroupIDs []string) (tagGroups []*tag.TagGroup, err error) {
	tagGroups = []*tag.TagGroup{}

	if len(arrayGroupIDs) > 0 {
		db = db.
			//Debug().
			Where("index_tag_group_id in (?)", arrayGroupIDs).
			Find(&tagGroups)
		err = db.Error
	}

	return tagGroups, err
}

func (srv *TagService) GetTagsByIDs(db *gorm.DB, arrayTagIDs []string) (tags []*tag.Tag, err error) {
	tags = []*tag.Tag{}

	if len(arrayTagIDs) > 0 {
		db = db.
			//Debug().
			Where("index_tag_id in (?)", arrayTagIDs).
			Find(&tags)
		err = db.Error
	}

	return tags, err
}

// ---------------------------------------------------------

// --- Customer ---
func (srv *TagService) AppendTagsToObject(db *gorm.DB, object database.ModelInterface, tags []*tag.Tag) (err error) {
	pivots, err := (&tag.RTagToObject{}).MakePivotsFromObjectAndTags(object, tags)
	if err != nil {
		return err
	}
	err = database.AppendMorphPivots(db, pivots)
	return err
}

func (srv *TagService) SyncTagsToObject(db *gorm.DB, object database.ModelInterface, tags []*tag.Tag) (err error) {
	pivots, err := (&tag.RTagToObject{}).MakePivotsFromObjectAndTags(object, tags)
	if err != nil {
		return err
	}
	err = database.SyncMorphPivots(db, pivots)
	return err
}

func (srv *TagService) ClearObjectTags(db *gorm.DB, obj database.ModelInterface) (err error) {
	err = database.ClearPivots(db, &modelWX.RWXTagToObject{
		TaggableOwnerType: object2.NewNullString(obj.GetTableName(true), true),
		TaggableObjectID:  object2.NewNullString(obj.GetForeignReferValue(), true),
	}, true, false)
	return err
}
