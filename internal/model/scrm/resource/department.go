package resource

import (
	"PowerX/internal/model"
	"PowerX/internal/model/powermodel"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WeWorkResource struct {
	powermodel.PowerModel

	Url          string `gorm:"comment:微信地址;column:url" json:"url"`
	FileName     string `gorm:"unique;comment:文件名;column:file_name" json:"file_name"`
	Remark       string `gorm:"comment:备注;column:remark" json:"remark"`
	BucketName   string `gorm:"comment:桶;column:bucket_name" json:"bucket_name"`
	Size         int    `gorm:"comment:大小;column:size" json:"size"`
	ResourceType string `gorm:"comment:资源类型：image,voice,file, video, other;column:resource_type" json:"resource_type"`
}

func (mdl *WeWorkResource) TableName() string {
	return model.PowerXSchema + "." + model.TableNameWeWorkResource
}

func (mdl *WeWorkResource) GetTableName(needFull bool) string {
	tableName := model.TableNameWeWorkResource
	if needFull {
		tableName = mdl.TableName()
	}
	return tableName
}

// Query
//
//	@Description:
//	@receiver e
//	@param db
//	@return departments
func (e WeWorkResource) Query(db *gorm.DB) (resources []*WeWorkResource) {

	err := db.Model(e).Find(&resources).Error
	if err != nil {
		panic(err)
	}
	return resources

}

// Action
//
//	@Description:
//	@receiver e
//	@param db
//	@param contacts
func (e *WeWorkResource) Action(db *gorm.DB, resources []*WeWorkResource) {

	err := db.Table(e.TableName()).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "file_name"}}, UpdateAll: true}).CreateInBatches(&resources, 100).Error
	if err != nil {
		panic(err)
	}

}
