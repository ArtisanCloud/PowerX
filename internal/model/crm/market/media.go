package market

import (
	"PowerX/internal/model"
	"PowerX/internal/model/media"
	"PowerX/internal/model/powermodel"
	"gorm.io/gorm"
)

type Media struct {
	powermodel.PowerModel

	PivotDetailImages []*media.PivotMediaResourceToObject `gorm:"polymorphic:Object;polymorphicValue:media" json:"pivotDetailImages"`
	CoverImage        *media.MediaResource                `gorm:"foreignKey:CoverImageId;references:Id" json:"coverImage"`

	Title        string `gorm:"comment:名称" json:"title"`
	SubTitle     string `gorm:"comment:副标题" json:"subTitle"`
	CoverImageId int64  `gorm:"comment:封面图Id" json:"coverImageId"`
	ResourceUrl  string `gorm:"comment:资源外链Url" json:"resourceUrl"`
	Description  string `gorm:"comment:描述" json:"description"`
	MediaType    int    `gorm:"comment:媒体类型" json:"mediaType"`
	ViewedCount  int    `gorm:"comment:浏览次数" json:"viewedCount"`
}

const MediaUniqueId = powermodel.UniqueId

func (mdl *Media) TableName() string {
	return model.PowerXSchema + "." + model.TableNameMedia
}

func (mdl *Media) GetTableName(needFull bool) string {
	tableName := model.TableNameMedia
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

const TypeMediaType = "_media_type"

const (
	MediaTypeProductShowcase      = "_product_showcase"       // 产品展示
	MediaTypeTutorialDemo         = "_tutorial_demo"          // 教程和演示
	MediaTypeCustomerReviews      = "_customer_reviews"       // 买家评价
	MediaTypeBrandStory           = "_brand_story"            // 品牌故事
	MediaTypePromotionalCampaigns = "_promotional_campaigns"  // 促销活动
	MediaTypeSocialMediaPromotion = "_social_media_promotion" // 社交媒体推广
	MediaTypeTrialSamples         = "_trial_samples"          // 试用和样品
	MediaTypeRecommendations      = "_recommendations"        // 推荐和搭配
	MediaTypeUserGeneratedContent = "_user_generated_content" // 用户生成内容
)

func (mdl *Media) ClearPivotDetailImages(db *gorm.DB) error {
	conditions := &map[string]interface{}{}
	(*conditions)[media.PivotMediaResourceToObjectOwnerKey] = model.TableNameMedia
	(*conditions)[media.PivotMediaResourceToObjectForeignKey] = mdl.Id

	return powermodel.ClearMorphPivots(db, &media.PivotMediaResourceToObject{}, false, false, conditions)
}
