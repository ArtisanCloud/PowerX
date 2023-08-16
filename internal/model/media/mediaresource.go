package media

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types"
)

type MediaResource struct {
	powermodel.PowerModel

	CustomerId    int64  `gorm:"comment:客户Id; index" json:"customerId"`
	Filename      string `gorm:"comment:名称" json:"filename"`
	Size          int64  `gorm:"comment:尺寸" json:"size"`
	Url           string `gorm:"comment:url" json:"url"`
	BucketName    string `gorm:"comment:Bucket名称" json:"bucketName"`
	IsLocalStored bool   `gorm:"comment:是否本地存储" json:"isLocalStored"`
	ContentType   string `gorm:"comment:内容类型" json:"contentType"`
	ResourceType  string `gorm:"comment:媒体类型" json:"mediaType"`
}

type MediaSet struct {
}

const MediaUsageCover = "_cover"
const MediaUsageDetail = "_detail"

func GetImageIds(pivots []*PivotMediaResourceToObject) ([]int64, []*types.SortIdItem) {
	arrayIds := []int64{}
	arrayIdSortIndexs := []*types.SortIdItem{}
	if len(pivots) <= 0 {
		return arrayIds, arrayIdSortIndexs
	}
	for _, pivot := range pivots {
		arrayIds = append(arrayIds, pivot.MediaResourceId)
		arrayIdSortIndexs = append(arrayIdSortIndexs, &types.SortIdItem{
			Id:        pivot.MediaResourceId,
			SortIndex: pivot.Sort,
		})
	}
	return arrayIds, arrayIdSortIndexs
}
