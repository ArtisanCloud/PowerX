package powermodel

import (
	"database/sql"
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

const PageDefaultSize = 20

var TablePrefix string

type ModelInterface interface {
	GetTableName(needFull bool) string
	GetPowerModel() ModelInterface
	GetID() int64
	GetUUID() string
	GetPrimaryKey() string
	GetForeignRefer() string
	GetForeignReferValue() int64
}

type PowerModel struct {
	Id int64 `gorm:"autoIncrement:true;unique; column:id; ->;<-:create" json:"id"`

	CreatedAt time.Time      `gorm:"column:created_at; ->;<-:create " json:"createdAt"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

type PowerUUIDModel struct {
	Id        int64          `gorm:"autoIncrement:true;unique; column:id; ->;<-:create" json:"-"`
	UUID      string         `gorm:"primaryKey;autoIncrement:false;unique; column:uuid; ->;<-:create " json:"uuid" sql:"index"`
	CreatedAt time.Time      `gorm:"column:created_at; ->;<-:create " json:"createdAt"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

const UniqueId = "id"
const UniqueUuid = "uuid"

const ModelStatusDraft int8 = 0
const ModelStatusActive int8 = 1
const ModelStatusCanceled int8 = 2
const ModelStatusPending int8 = 3
const ModelStatusInactive int8 = 4

var ArrayModelFields *object.HashMap = &object.HashMap{}

func NewPowerUUIDModel() *PowerUUIDModel {
	now := time.Now()
	return &PowerUUIDModel{
		UUID:      uuid.New().String(),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func NewPowerModel() *PowerModel {
	now := time.Now()
	return &PowerModel{
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (mdl *PowerUUIDModel) GetID() int64 {
	return mdl.Id
}

func (mdl *PowerUUIDModel) GetTableName(needFull bool) string {
	return ""
}

func (mdl *PowerUUIDModel) GetPowerModel() ModelInterface {
	return mdl
}

func (mdl *PowerUUIDModel) GetUUID() string {
	return mdl.UUID
}

func (mdl *PowerUUIDModel) GetPrimaryKey() string {
	return "uuid"
}

func (mdl *PowerUUIDModel) GetForeignRefer() string {
	return "uuid"
}
func (mdl *PowerUUIDModel) GetForeignReferValue() int64 {
	return mdl.Id
}

// ---------------------------------------------------------------------------------------------------------------------
// PowerModel
// ---------------------------------------------------------------------------------------------------------------------

func (mdl *PowerModel) GetTableName(needFull bool) string {
	return ""
}

func (mdl *PowerModel) GetPowerModel() ModelInterface {
	return mdl
}

func (mdl *PowerModel) GetID() int64 {
	return mdl.Id
}
func (mdl *PowerModel) GetUUID() string {
	return ""
}

func (mdl *PowerModel) GetPrimaryKey() string {
	return "id"
}

func (mdl *PowerModel) GetForeignRefer() string {
	return "id"
}
func (mdl *PowerModel) GetForeignReferValue() int64 {
	return mdl.Id
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
		pageSize = PageDefaultSize
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
	models interface{}, fieldsToUpdate []string, withAssociations bool) error {

	db = db.Model(mdl)

	if len(fieldsToUpdate) <= 0 {
		fieldsToUpdate = GetModelFields(mdl)
	}

	if !withAssociations {
		db = db.Omit(clause.Associations)
	}

	result := db.
		//Debug().
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
