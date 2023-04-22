package powermodel

import (
	fmt2 "PowerX/pkg/printx"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type PivotInterface interface {
	ModelInterface
	GetForeignKey() string
	GetForeignValue() int64
	GetJoinKey() string
	GetJoinValue() int64
	GetOwnerKey() string
	GetOwnerValue() int64
	GetPivotComposedUniqueID() string
}

type PowerPivot struct {
	Id        int64     `gorm:"AUTO_INCREMENT;PRIMARY_KEY;not null" json:"id"`
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
func (mdl *PowerPivot) GetID() int64 {
	return mdl.Id
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
func (mdl *PowerPivot) GetForeignReferValue() int64 {
	return mdl.Id
}

func (mdl *PowerPivot) GetForeignKey() string {
	return "foreign_key"
}
func (mdl *PowerPivot) GetForeignValue() int64 {
	return 0
}

func (mdl *PowerPivot) GetJoinKey() string {
	return "join_key"
}
func (mdl *PowerPivot) GetJoinValue() int64 {
	return 0
}

func (mdl *PowerPivot) GetOwnerKey() string {
	return "owner_key"
}
func (mdl *PowerPivot) GetOwnerValue() int64 {
	return 0
}

func (mdl *PowerPivot) GetPivotComposedUniqueID() string {
	return fmt.Sprintf("%d-%d-%d", mdl.GetOwnerValue(), mdl.GetForeignValue(), mdl.GetJoinValue())
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

			result = SelectMorphPivot(db, pivots[i], nil)
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
		fmt2.Dump("pivots is empty")
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

func SelectPivots(db *gorm.DB, pivot PivotInterface, byForeignKey bool, byJoinKey bool, where *map[string]interface{}) (result *gorm.DB) {

	return SelectMorphPivots(db, pivot, byForeignKey, byJoinKey, where)
}

func SelectPivot(db *gorm.DB, pivot PivotInterface) (result *gorm.DB) {

	return SelectMorphPivot(db, pivot, nil)
}

// --------------------------------------------------------------------

// select many pivots with foreign key
func SelectMorphPivots(db *gorm.DB, pivot PivotInterface, byForeignKey bool, byJoinKey bool, where *map[string]interface{}) *gorm.DB {

	db = db.
		//Debug().
		Model(pivot)

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

	if where != nil {
		db = db.Where(where)
	}

	// join foreign type if exists
	if pivot.GetOwnerValue() != 0 {
		db = db.Where(pivot.GetOwnerKey(), pivot.GetOwnerValue())
	}

	return db
}

// select one pivot with foreign key and join key
func SelectMorphPivot(db *gorm.DB, pivot PivotInterface, where *map[string]interface{}) (result *gorm.DB) {

	result = SelectMorphPivots(db, pivot, true, true, where)

	return result
}

func UpsertPivots(db *gorm.DB, uniqueName string, pivots []PivotInterface, fieldsToUpdate []string) error {

	if len(pivots) <= 0 {
		fmt2.Dump("pivots is empty")
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
