package main

import (
	"fmt"
	"github.com/ArtisanCloud/PowerLibs/v2/object"
	"github.com/ArtisanCloud/PowerX/app/models"
	"github.com/ArtisanCloud/PowerX/app/service"
	globalDatabase "github.com/ArtisanCloud/PowerX/database/global"
	"github.com/gin-gonic/gin"
	"reflect"
)

type WXContactWayGroupTableSeeder struct {
	SeederInterface
	ServiceWXContactWayGroup *service.ContactWayGroupService
}

const SEED_WX_CONTACT_WAY_UUID = "wxContactWayGroup-default-uuid"

func NewWXContactWayGroupTableSeeder(ctx *gin.Context) *WXContactWayGroupTableSeeder {
	return &WXContactWayGroupTableSeeder{
		ServiceWXContactWayGroup: service.NewContactWayGroupService(ctx),
	}
}

func (seeder *WXContactWayGroupTableSeeder) Run(ctx *gin.Context) (err error) {
	seederName := reflect.TypeOf(seeder).String()

	err = seeder.SeedWXContactWayForEvent()
	if err != nil {
		fmt.Printf("seed %s for default contact way group, error:%s \n", seederName, err.Error())
	}
	fmt.Printf("seed %s for default contact way group done \n\n", seederName)

	return err
}

func (seeder *WXContactWayGroupTableSeeder) SeedWXContactWayForEvent() (err error) {
	wxContactWayGroup := models.NewContactWayGroup(object.NewCollection(&object.HashMap{
		"name": "默认分组",
	}))
	wxContactWayGroup.UUID = SEED_WX_CONTACT_WAY_UUID

	wxContactWayGroup, err = seeder.ServiceWXContactWayGroup.UpsertContactWayGroup(globalDatabase.G_DBConnection, wxContactWayGroup, false)

	return err
}
