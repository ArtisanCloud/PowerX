package powerx

import (
	"PowerX/internal/config"
	"PowerX/internal/model/media"
	fmt "PowerX/pkg/printx"
	"context"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"mime/multipart"
)

// MediaResourceUseCase Use Case
type MediaResourceUseCase struct {
	db        *gorm.DB
	OSSClient *minio.Client
}

const BucketProduct = "bucket.product"

func NewMediaResourceUseCase(db *gorm.DB, conf *config.Config) *MediaResourceUseCase {
	// 使用Minio API SDK
	c, err := minio.New(conf.OSS.Minio.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(conf.OSS.Minio.Credentials.AccessKey, conf.OSS.Minio.Credentials.SecretKey, ""),
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

func (uc *MediaResourceUseCase) CreateProductResource(ctx context.Context, handle *multipart.FileHeader) (*media.MediaResource, error) {

	return uc.CreateResource(ctx, BucketProduct, handle)
}

func (uc *MediaResourceUseCase) CreateResource(ctx context.Context, bucket string, handle *multipart.FileHeader) (*media.MediaResource, error) {
	exist, err := uc.OSSClient.BucketExists(ctx, bucket)
	if err != nil {
		return nil, err
	}

	if !exist {
		location := "us-east-1"
		err = uc.OSSClient.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: location})
		if err != nil {
			return nil, err
		}
		// 设置存储桶策略
		policy := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Sid": "PublicReadGetObject",
				"Effect": "Allow",
				"Principal": "*",
				"Action": ["s3:GetObject"],
				"Resource": ["arn:aws:s3:::` + bucket + `/*"]
			}
		]
	}`
		err = uc.OSSClient.SetBucketPolicy(context.Background(), bucket, policy)
		if err != nil {
			return nil, err
		}
	}

	// Upload the resource file
	objectName := handle.Filename
	filePath, _ := handle.Open()
	contentType := handle.Header.Get("Content-Type")
	info, err := uc.OSSClient.PutObject(ctx, bucket, objectName, filePath, handle.Size, minio.PutObjectOptions{ContentType: contentType})
	fmt.Dump(info)

	return nil, err
}
