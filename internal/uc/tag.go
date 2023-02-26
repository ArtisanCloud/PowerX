package uc

import (
	"PowerX/internal/types"
	"context"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TagUseCase struct {
	db *gorm.DB
}

func newTagUseCase(db *gorm.DB) *TagUseCase {
	return &TagUseCase{
		db: db,
	}
}

type Tag struct {
	Name string
	Desc string
	*types.Model
}

type TagGroup struct {
	TagId     int64
	GroupName string
	Order     int32
	*types.Model
}

func (t *TagUseCase) CreateTags(ctx context.Context, tags []*Tag) {
	err := t.db.WithContext(ctx).Model(&Tag{}).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoNothing: true,
	}).Create(&tags).Error
	if err != nil {
		panic(errors.Wrap(err, "create tags failed"))
	}
}
