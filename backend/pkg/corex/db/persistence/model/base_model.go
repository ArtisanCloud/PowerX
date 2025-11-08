package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PowerModel struct {
	//ID uint64 `gorm:"autoIncrement:true;unique; column:id; ->;<-:create" json:"id"`
	ID uint64 `gorm:"primaryKey;autoIncrement;column:id" json:"id"`

	CreatedAt time.Time      `gorm:"column:created_at; ->;<-:create " json:"createdAt"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

type PowerUUIDModel struct {
	//ID        uint64         `gorm:"autoIncrement:true;unique; column:id; ->;<-:create" json:"-"`
	ID uint64 `gorm:"primaryKey;autoIncrement;column:id" json:"id"`

	//UUID      uuid.UUID      `gorm:"type:uuid;primaryKey;autoIncrement:false;unique; column:uuid; ->;<-:create " json:"uuid" sql:"index"`
	UUID uuid.UUID `gorm:"type:uuid;primaryKey;column:uuid;index" json:"uuid"`

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

func (m *PowerUUIDModel) BeforeCreate(tx *gorm.DB) error {
	if m.UUID == uuid.Nil {
		m.UUID = uuid.New()
	}
	return nil
}

type Pagination struct {
	Limit      int         `json:"limit"`
	Page       int         `json:"page"`
	Sort       string      `json:"sort"`
	TotalRows  int64       `json:"totalRows"`
	TotalPages int         `json:"totalPages"`
	Data       interface{} `json:"data"`
}

const DefaultPageSize = 10
const MaxPageSize = 999
