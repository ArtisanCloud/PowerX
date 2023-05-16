package media

import "PowerX/internal/model/powermodel"

type MediaResource struct {
	powermodel.PowerModel

	Filename      string `gorm:"comment:名称" json:"filename"`
	Size          int64  `gorm:"comment:尺寸" json:"size"`
	Url           string `gorm:"comment:url" json:"url"`
	BucketName    string `gorm:"comment:Bucket名称" json:"bucketName"`
	IsLocalStored bool   `gorm:"comment:是否本地存储" json:"isLocalStored"`
	ContentType   string `gorm:"comment:内容类型" json:"contentType"`
	ResourceType  string `gorm:"comment:媒体类型" json:"mediaType"`
}
