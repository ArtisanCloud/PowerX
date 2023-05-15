package media

import "PowerX/internal/model/powermodel"

type MediaResource struct {
	powermodel.PowerModel

	Filename     string `gorm:"comment:名称" json:"filename"`
	Size         int64  `gorm:"comment:尺寸" json:"size"`
	Url          string `gorm:"comment:url" json:"url"`
	ContentType  string `gorm:"comment:内容类型" json:"contentType"`
	ResourceType string `gorm:"comment:媒体名称" json:"mediaType"`
}
