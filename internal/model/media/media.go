package media

type Media struct {
	Title       string `gorm:"comment:名称" json:"title"`
	SubTitle    string `gorm:"comment:副标题" json:"subTitle"`
	CoverUrl    string `gorm:"comment:封面Url" json:"coverUrl"`
	ResourceUrl string `gorm:"comment:资源外链Url" json:"resourceUrl"`
	Description string `gorm:"comment:描述" json:"description"`
	MediaType   int    `gorm:"comment:媒体类型" json:"mediaType"`
	ViewedCount int    `gorm:"comment:名称" json:"viewedCount"`
}
