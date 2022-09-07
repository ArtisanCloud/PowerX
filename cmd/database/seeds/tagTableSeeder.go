package main

import (
	"fmt"
	"github.com/ArtisanCloud/PowerLibs/v2/database/tag"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	service "github.com/ArtisanCloud/PowerX/app/service"
	"github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"reflect"
)

type TagTableSeeder struct {
	SeederInterface
	ServiceTag *service.TagService
}

const SEED_TAG_UUID = "tag-uuid"

func NewTagTableSeeder(ctx *gin.Context) *TagTableSeeder {
	return &TagTableSeeder{
		ServiceTag: service.NewTagService(ctx),
	}
}

func (seeder *TagTableSeeder) Run(ctx *gin.Context) (err error) {

	seederName := reflect.TypeOf(seeder).String()

	serviceTag := service.NewTagService(nil)

	arrayTags := []string{
		"新客户",
		"初步沟通",
		"意向客户",
		"购买成功客户",
		"无意向客户",
	}

	err = global.G_DBConnection.Transaction(func(tx *gorm.DB) error {
		tags := []*tag.Tag{}
		for _, strTag := range arrayTags {
			tag := tag.NewTag(object.NewCollection(&object.HashMap{
				"name":          strTag,
				"parentTagUUID": "",
				"type":          tag.TAG_TYPE_STAGE,
			}))
			tag.UniqueID = tag.GetComposedUniqueID()

			tags = append(tags, tag)

			if err != nil {
				return err
			}
		}

		err = serviceTag.UpsertTags(global.G_DBConnection, tags, nil)
		return nil
	})

	if err != nil {
		fmt.Printf("seed %s , error:%s \n", seederName, err.Error())
	}

	fmt.Printf("seed %s done \n\n", seederName)

	return err
}
