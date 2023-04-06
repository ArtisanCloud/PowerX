package powermodel

import (
	"database/sql"
	"fmt"
	"github.com/ArtisanCloud/PowerLibs/v3/object"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
	"math"
	"reflect"
	"sync"
	"time"
)

const PAGE_DEFAULT_SIZE = 20

var TABLE_PREFIX string

type ModelInterface interface {
	GetTableName(needFull bool) string
	GetPowerModel() ModelInterface
	GetID() int32
	GetUUID() string
	GetPrimaryKey() string
	GetForeignRefer() string
	GetForeignReferValue() string
}

type PowerModel struct {
	ID   int32  `gorm:"autoIncrement:true;unique; column:id; ->;<-:create" json:"-"`
	UUID string `gorm:"primaryKey;autoIncrement:false;unique; column:uuid; ->;<-:create " json:"uuid" sql:"index"`

	CreatedAt time.Time      `gorm:"column:created_at; ->;<-:create " json:"createdAt"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

type PowerCompactModel struct {
	ID int32 `gorm:"primaryKey;autoIncrement:true;unique; column:id; ->;<-:create" json:"-"`

	CreatedAt time.Time      `gorm:"column:created_at; ->;<-:create " json:"createdAt"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

const UNIQUE_ID = "uuid"
const COMPACT_UNIQUE_ID = "id"

const MODEL_STATUS_DRAFT int8 = 0
const MODEL_STATUS_ACTIVE int8 = 1
const MODEL_STATUS_CANCELED int8 = 2
const MODEL_STATUS_PENDING int8 = 3
const MODEL_STATUS_INACTIVE int8 = 4

const APPROVAL_STATUS_DRAFT int8 = 0
const APPROVAL_STATUS_PENDING int8 = 1
const APPROVAL_STATUS_APPROVED int8 = 3
const APPROVAL_STATUS_REJECTED int8 = 4

var ArrayModelFields *object.HashMap = &object.HashMap{}

func NewPowerModel() *PowerModel {
	now := time.Now()
	return &PowerModel{
		UUID:      uuid.New().String(),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func NewPowerCompactModel() *PowerCompactModel {
	now := time.Now()
	return &PowerCompactModel{
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (mdl *PowerModel) GetID() int32 {
	return mdl.ID
}

func (mdl *PowerModel) GetTableName(needFull bool) string {
	return ""
}

func (mdl *PowerModel) GetPowerModel() ModelInterface {
	return mdl
}

func (mdl *PowerModel) GetUUID() string {
	return mdl.UUID
}

func (mdl *PowerModel) GetPrimaryKey() string {
	return "uuid"
}

func (mdl *PowerModel) GetForeignRefer() string {
	return "uuid"
}
func (mdl *PowerModel) GetForeignReferValue() string {
	return mdl.UUID
}

// ---------------------------------------------------------------------------------------------------------------------
// PowerCompactModel
// ---------------------------------------------------------------------------------------------------------------------

func (mdl *PowerCompactModel) GetTableName(needFull bool) string {
	return ""
}

func (mdl *PowerCompactModel) GetPowerModel() ModelInterface {
	return mdl
}

func (mdl *PowerCompactModel) GetID() int32 {
	return mdl.ID
}
func (mdl *PowerCompactModel) GetUUID() string {
	return ""
}

func (mdl *PowerCompactModel) GetPrimaryKey() string {
	return "id"
}

func (mdl *PowerCompactModel) GetForeignRefer() string {
	return "id"
}
func (mdl *PowerCompactModel) GetForeignReferValue() string {
	return fmt.Sprintf("%d", mdl.ID)
}

/**
 * Scope Where Conditions
 */
func WhereUUID(uuid string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("uuid=?", uuid)
	}
}

func WhereAccountUUID(uuid string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("account_uuid=@value", sql.Named("value", uuid))
	}
}

func GetFirst(db *gorm.DB, conditions *map[string]interface{}, model interface{}, preloads []string) (err error) {

	if conditions != nil {
		db = db.Where(*conditions)
	}

	// add preloads
	if len(preloads) > 0 {
		for _, preload := range preloads {
			if preload != "" {
				db.Preload(preload)
			}
		}
	}

	result := db.First(model)

	return result.Error
}

func GetList(db *gorm.DB, conditions *map[string]interface{},
	models interface{}, preloads []string,
	page int, pageSize int) (paginator *Pagination, err error) {

	if page < 0 {
		page = 0
	}
	if pageSize <= 0 {
		pageSize = PAGE_DEFAULT_SIZE
	}

	// add pagination
	paginator = NewPagination(page, pageSize, "")
	var totalRows int64
	db.Model(models).Count(&totalRows)
	paginator.TotalRows = totalRows
	totalPages := int(math.Ceil(float64(totalRows) / float64(paginator.Limit)))
	paginator.TotalPages = totalPages

	db = db.Scopes(
		Paginate(page, pageSize),
	)

	if conditions != nil {
		db = db.Where(*conditions)
	}

	// add preloads
	if len(preloads) > 0 {
		for _, preload := range preloads {
			if preload != "" {
				db.Preload(preload)
			}
		}
	}

	// chunk datas
	result := db.Find(models)
	err = result.Error
	if err != nil {
		return paginator, err
	}

	paginator.Data = models

	return paginator, nil
}

func GetAllList(db *gorm.DB, conditions *map[string]interface{},
	items interface{}, preloads []string) (err error) {

	if conditions != nil {
		db = db.Where(*conditions)
	}

	// add preloads
	if len(preloads) > 0 {
		for _, preload := range preloads {
			if preload != "" {
				db = db.Preload(preload)
			}
		}
	}

	// chunk datas
	result := db.
		//Debug().
		Order("id ASC").
		Find(items)
	err = result.Error
	if err != nil {
		return err
	}

	return nil
}

func InsertModelsOnUniqueID(db *gorm.DB, mdl interface{}, uniqueName string, models interface{}) error {

	result := db.Model(mdl).
		//Debug().
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: uniqueName}},
			DoNothing: true,
		}).Create(models)

	return result.Error
}

func UpsertModelsOnUniqueID(db *gorm.DB, mdl interface{}, uniqueName string,
	models interface{}, fieldsToUpdate []string) error {

	if len(fieldsToUpdate) <= 0 {
		fieldsToUpdate = GetModelFields(mdl)
	}

	result := db.Model(mdl).
		//Debug().
		Omit(clause.Associations).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: uniqueName}},
			DoUpdates: clause.AssignmentColumns(fieldsToUpdate),
		}).
		Create(models)

	return result.Error
}

/**
 * model methods
 */

func GetTableFullName(schema string, prefix string, tableName string) (fullName string) {

	fullName = schema + "." + prefix + tableName

	return fullName
}

func GetModelFields(model interface{}) (fields []string) {

	// check if it has been loaded
	modelType := reflect.TypeOf(model)
	modelName := modelType.String()
	if (*ArrayModelFields)[modelName] != nil {
		return (*ArrayModelFields)[modelName].([]string)
	}

	//fmt.Printf("parse object ~%s~ model fields \n", modelName)
	gormSchema, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		println(err)
		return fields
	}

	fields = []string{}
	for _, field := range gormSchema.Fields {
		if field.DBName != "" && !field.PrimaryKey && !field.Unique && field.Updatable {
			fields = append(fields, field.DBName)
		}
	}
	(*ArrayModelFields)[modelName] = fields
	//fmt.Printf("parsed object ~%s~ model fields and fields count is %d \n\n", modelName, len(fields))

	return fields
}

func GetModelFieldValues(model interface{}) (mapFields *object.HashMap, err error) {

	//fmt.Printf("parse object ~%s~ model fields \n", modelName)
	gormSchema, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		println(err)
		return mapFields, err
	}

	mapFields = &object.HashMap{}
	for _, field := range gormSchema.Fields {
		if field.DBName != "" && !field.PrimaryKey && !field.Unique && field.Updatable {
			(*mapFields)[field.DBName] = field.ValueOf
		}
	}

	return mapFields, err
}

func IsPowerModelLoaded(mdl ModelInterface) bool {
	if object.IsObjectNil(mdl) {
		return false
	}

	myModel := mdl.GetPowerModel()
	if object.IsObjectNil(myModel) {
		return false
	}

	if mdl.GetUUID() == "" {
		return false
	}

	return true
}

func IsPowerPivotLoaded(mdl ModelInterface) bool {

	if object.IsObjectNil(mdl) {
		return false
	}

	myModel := mdl.GetPowerModel()
	if object.IsObjectNil(myModel) {
		return false
	}

	if mdl.GetID() > 0 {
		return false
	}

	return true
}

// ---------------------------------------------------------------------------------------------------------------------
func FormatJsonBArrayToWhereInSQL(fields string, arrayValues []string) (sqlWhere string) {

	if fields == "" || len(arrayValues) <= 0 {
		return ""
	}

	sqlWhere = fields + " ?| array["
	for _, value := range arrayValues {
		if value != "" {
			sqlWhere += "'" + value + "',"
		}
	}
	sqlWhere = sqlWhere[0:len(sqlWhere)-1] + "]"

	return sqlWhere
}
