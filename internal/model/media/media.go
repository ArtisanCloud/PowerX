package media

import (
	"PowerX/internal/model/powermodel"
	"gorm.io/gorm"
)

type Media struct {
	powermodel.PowerModel

	PivotDetailImages []*PivotMediaResourceToObject `gorm:"polymorphic:Object;polymorphicValue:media" json:"pivotDetailImages"`
	CoverImage        *MediaResource                `gorm:"foreignKey:CoverImageId;references:Id" json:"coverImage"`

	Title        string    `gorm:"comment:名称" json:"title"`
	SubTitle     string    `gorm:"comment:副标题" json:"subTitle"`
	CoverImageId int64     `gorm:"comment:封面图Id" json:"coverImageId"`
	ResourceUrl  string    `gorm:"comment:资源外链Url" json:"resourceUrl"`
	Description  string    `gorm:"comment:描述" json:"description"`
	MediaType    MediaType `gorm:"comment:媒体类型" json:"mediaType"`
	ViewedCount  int       `gorm:"comment:浏览次数" json:"viewedCount"`
}

type MediaType int8

const TableNameMedia = "media"
const MediaUniqueId = powermodel.UniqueId

const (
	MediaTypeProductShowcase      MediaType = iota // 产品展示
	MediaTypeTutorialDemo                          // 教程和演示
	MediaTypeCustomerReviews                       // 买家评价
	MediaTypeBrandStory                            // 品牌故事
	MediaTypePromotionalCampaigns                  // 促销活动
	MediaTypeSocialMediaPromotion                  // 社交媒体推广
	MediaTypeTrialSamples                          // 试用和样品
	MediaTypeRecommendations                       // 推荐和搭配
	MediaTypeUserGeneratedContent                  // 用户生成内容
)

func (mdl *Media) ClearPivotDetailImages(db *gorm.DB) error {
	conditions := &map[string]interface{}{}
	(*conditions)[PivotMediaResourceToObjectOwnerKey] = TableNameMedia
	(*conditions)[PivotMediaResourceToObjectForeignKey] = mdl.Id

	return powermodel.ClearMorphPivots(db, &PivotMediaResourceToObject{}, false, false, conditions)
}
