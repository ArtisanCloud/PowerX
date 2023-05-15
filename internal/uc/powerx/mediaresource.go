package powerx

import (
	"PowerX/internal/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// MediaResourceUseCase Use Case
type MediaResourceUseCase struct {
	db        *gorm.DB
	OSSClient *minio.Client
}

func NewMediaResourceUseCase(db *gorm.DB, conf *config.Config) *MediaResourceUseCase {
	// 使用Minio API SDK
	c, err := minio.New(conf.OSS.Minio.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(conf.OSS.Minio.Credentials.SecretKey, conf.OSS.Minio.Credentials.SecretKey, ""),
		Secure: conf.OSS.Minio.UseSSL,
	})

	if err != nil {
		panic(errors.Wrap(err, "wework init failed"))
	}

	return &MediaResourceUseCase{
		db:        db,
		OSSClient: c,
	}
}
