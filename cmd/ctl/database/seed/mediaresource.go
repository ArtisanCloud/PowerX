package seed

import (
	"PowerX/internal/config"
	"PowerX/internal/model/media"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/uc/powerx"
	"PowerX/pkg/httpx"
	"PowerX/pkg/securityx"
	"PowerX/pkg/slicex"
	"fmt"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	path2 "path"
)

func CreateMediaResources(db *gorm.DB, conf *config.Config) (err error) {

	var count int64
	if err = db.Model(&media.MediaResource{}).Count(&count).Error; err != nil {
		panic(errors.Wrap(err, "init media resource failed"))
	}

	data := DefaultMediaResource(db, conf)
	if count == 0 {
		for _, item := range data {
			if err = db.Model(&media.MediaResource{}).Create(item).Error; err != nil {
				panic(errors.Wrap(err, "init media resource failed"))
			}
		}
	}
	return err
}

func DefaultMediaResource(db *gorm.DB, conf *config.Config) (data []*media.MediaResource) {
	data = []*media.MediaResource{}
	ucMediaResource := powerx.NewMediaResourceUseCase(db, conf)

	path := path2.Join(powerx.DefaultStoragePath, "shop")
	coverImage := ProductCoverImage(ucMediaResource.LocalStorageUrl, path)
	detailImages := ProductDetailImages(ucMediaResource.LocalStorageUrl, path)
	//fmt2.Dump(coverImage, detailImages)

	data = slicex.Concatenate(data, coverImage, detailImages)
	return data
}

func ProductCoverImage(url string, name string) []*media.MediaResource {
	imageUrl, _ := httpx.AppendURIs(url, fmt.Sprintf("%s/0.png", name))
	return []*media.MediaResource{
		{
			PowerUUIDModel: powermodel.PowerUUIDModel{
				UUID: securityx.GenerateUUID(),
			},
			Url:           imageUrl,
			IsLocalStored: true,
		},
	}
}

func ProductDetailImages(url string, path string) []*media.MediaResource {
	urls := []*media.MediaResource{}
	for i := 0; i < 20; i++ {
		imageUrl, _ := httpx.AppendURIs(url, path, fmt.Sprintf("%d.png", i+1))

		urls = append(urls, &media.MediaResource{
			PowerUUIDModel: powermodel.PowerUUIDModel{
				UUID: securityx.GenerateUUID(),
			},
			Url:           imageUrl,
			IsLocalStored: true,
		})
	}
	return urls
}
