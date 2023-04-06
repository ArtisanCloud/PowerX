package powermodel

import (
	"errors"
	fmt2 "fmt"
	"github.com/ArtisanCloud/PowerLibs/v3/fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type PivotInterface interface {
	ModelInterface
	GetForeignKey() string
	GetForeignValue() string
	GetJoinKey() string
	GetJoinValue() string
	GetOwnerKey() string
	GetOwnerValue() string
	GetPivotComposedUniqueID() string
}

type PowerPivot struct {
	ID        int32     `gorm:"AUTO_INCREMENT;PRIMARY_KEY;not null" json:"id"`
	CreatedAt time.Time `gorm:"column:created_at; ->;<-:create " json:"createdAt"`
	UpdatedAt time.Time `gorm:"column:updated_at" json:"updatedAt"`
}

func NewPowerPivot() *PowerPivot {
	now := time.Now()
	return &PowerPivot{
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// --------------------------------------------------------------------
func (mdl *PowerPivot) GetTableName(needFull bool) string {
	return ""
}

func (mdl *PowerPivot) GetPowerModel() ModelInterface {
	return mdl
}
func (mdl *PowerPivot) GetID() int32 {
	return mdl.ID
}

func (mdl *PowerPivot) GetUUID() string {
	return ""
}

func (mdl *PowerPivot) GetPrimaryKey() string {
	return "id"
}
func (mdl *PowerPivot) GetForeignRefer() string {
	return "id"
}
func (mdl *PowerPivot) GetForeignReferValue() string {
	return fmt2.Sprintf("%d", mdl.ID)
}

func (mdl *PowerPivot) GetForeignKey() string {
	return "foreign_key"
}
func (mdl *PowerPivot) GetForeignValue() string {
	return "foreign_id"
}

func (mdl *PowerPivot) GetJoinKey() string {
	return "join_key"
}
func (mdl *PowerPivot) GetJoinValue() string {
	return "join_id"
}

func (mdl *PowerPivot) GetOwnerKey() string {
	return "owner_key"
}
func (mdl *PowerPivot) GetOwnerValue() string {
	return ""
}

func (mdl *PowerPivot) GetPivotComposedUniqueID() string {
	return mdl.GetOwnerValue() + "-" + mdl.GetForeignValue() + "-" + mdl.GetJoinValue()
}

/**
 * Association Pivot
 */
func AssociationRelationship(db *gorm.DB, conditions *map[string]interface{}, mdl interface{}, relationship string, withClauseAssociations bool) *gorm.Association {

	tx := db.
		//Debug().
		Model(mdl)

	if withClauseAssociations {
		tx.Preload(clause.Associations)
	}

	if conditions != nil {
		tx = tx.Where(*conditions)
	}

	return tx.Association(relationship)
}

func ClearAssociations(db *gorm.DB, object ModelInterface, foreignKey string, pivot PivotInterface) error {
	result := db.Exec("DELETE FROM "+pivot.GetTableName(true)+" WHERE "+foreignKey+"=?", object.GetID())
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// --------------------------------------------------------------------

func AppendMorphPivots(db *gorm.DB, pivots []PivotInterface) (err error) {

	var result = &gorm.DB{}

	err = db.Transaction(func(tx *gorm.DB) error {
		for i := 0; i < len(pivots); i++ {

			result = SelectMorphPivot(db, pivots[i])
			if result.Error != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}

			//err = UpsertPivots(db, pivots[i].GetPivotComposedUniqueID(), []PivotInterface{pivots[i]}, nil)

			if result.RowsAffected == 0 || result.Error == gorm.ErrRecordNotFound {
				err = SavePivot(db, pivots[i])
				if err != nil {
					return err
				}
			} else {
				err = UpdatePivot(db, pivots[i])
				if err != nil {
					return err
				}
			}
		}
		return result.Error
	})

	return err
}

func SyncMorphPivots(db *gorm.DB, pivots []PivotInterface) (err error) {
	if len(pivots) <= 0 {
		fmt.Dump("pivots is empty")
		return nil
	}
	err = db.Transaction(func(tx *gorm.DB) error {

		err = ClearPivots(db, pivots[0], true, false)
		if err != nil {
			return err
		}

		err = AppendMorphPivots(db, pivots)

		return err
	})

	return err
}

// --------------------------------------------------------------------
func AppendPivots(db *gorm.DB, pivots []PivotInterface) (err error) {

	return AppendMorphPivots(db, pivots)
}
func SyncPivots(db *gorm.DB, pivots []PivotInterface) (err error) {

	return SyncMorphPivots(db, pivots)
}

func SelectPivots(db *gorm.DB, pivot PivotInterface, byForeignKey bool, byJoinKey bool) (result *gorm.DB) {

	return SelectMorphPivots(db, pivot, byForeignKey, byJoinKey)
}

func SelectPivot(db *gorm.DB, pivot PivotInterface) (result *gorm.DB) {

	return SelectMorphPivot(db, pivot)
}

// --------------------------------------------------------------------

// select many pivots with foreign key
func SelectMorphPivots(db *gorm.DB, pivot PivotInterface, byForeignKey bool, byJoinKey bool) (result *gorm.DB) {

	db = db.
		//Debug().
		Model(pivot)

	if byForeignKey && byJoinKey {
		// select via foreign key and join key
		result = db.
			Where(pivot.GetJoinKey(), pivot.GetJoinValue()).
			Where(pivot.GetForeignKey(), pivot.GetForeignValue())
	} else if byJoinKey {
		// select via join key
		result = db.Where(pivot.GetJoinKey(), pivot.GetJoinValue())
	} else {
		// select via foreign key
		result = db.Where(pivot.GetForeignKey(), pivot.GetForeignValue())
	}

	// join foreign type if exists
	if pivot.GetOwnerValue() != "" {
		db = db.Where(pivot.GetOwnerKey(), pivot.GetOwnerValue())
	}

	return result
}

// select one pivot with foreign key and join key
func SelectMorphPivot(db *gorm.DB, pivot PivotInterface) (result *gorm.DB) {

	result = SelectMorphPivots(db, pivot, true, true)

	return result
}

func UpsertPivots(db *gorm.DB, uniqueName string, pivots []PivotInterface, fieldsToUpdate []string) error {

	if len(pivots) <= 0 {
		fmt.Dump("pivots is empty")
		return nil
	}

	if len(fieldsToUpdate) <= 0 {
		fieldsToUpdate = GetModelFields(&pivots[0])
	}

	result := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: uniqueName}},
		DoUpdates: clause.AssignmentColumns(fieldsToUpdate),
	}).
		//Debug().
		Create(&pivots)

	return result.Error
}

func SavePivot(db *gorm.DB, pivot PivotInterface) error {
	result := db.
		//Debug().
		Create(pivot)

	return result.Error
}

func UpdatePivot(db *gorm.DB, pivot PivotInterface) error {
	result := db.
		//Debug().
		Save(pivot)

	return result.Error
}

// clear all pivots with foreign key and value
func ClearPivots(db *gorm.DB, pivot PivotInterface, byForeignKey bool, byJoinKey bool) (err error) {

	if byForeignKey && byJoinKey {
		// select via foreign key and join key
		db = db.
			Where(pivot.GetJoinKey(), pivot.GetJoinValue()).
			Where(pivot.GetForeignKey(), pivot.GetForeignValue())
	} else if byJoinKey {
		// select via join key
		db = db.Where(pivot.GetJoinKey(), pivot.GetJoinValue())
	} else {
		// select via foreign key
		db = db.Where(pivot.GetForeignKey(), pivot.GetForeignValue())
	}

	result := db.
		Debug().
		Delete(pivot)

	if result.Error != nil {
		return result.Error
	}

	return nil

}
