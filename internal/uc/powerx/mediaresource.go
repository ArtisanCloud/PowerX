package powerx

import (
	"PowerX/internal/config"
	"PowerX/internal/model/media"
	"PowerX/internal/types"
	"PowerX/pkg/filex"
	"PowerX/pkg/httpx"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"github.com/ArtisanCloud/PowerLibs/v3/cache"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
	"gorm.io/gorm"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const DefaultStoragePath = "resource/static"

// 分片上传配置常量
const (
	MultipartPartSize    = 100 * 1024 * 1024 // 100MB
	MultipartConcurrency = 5
	MultipartMaxRetries  = 3
)

// MediaResourceUseCase Use Case
type MediaResourceUseCase struct {
	db               *gorm.DB
	OSSClient        *minio.Client
	LocalStoragePath string
	LocalStorageUrl  string
}

const BucketMediaResourceProduct = "bucket.product"
const BucketMediaResourceVideo = "bucket.video"

func NewMediaResourceUseCase(db *gorm.DB, conf *config.Config) *MediaResourceUseCase {
	// 使用Minio API SDK
	var c *minio.Client
	var err error
	localStoragePath := DefaultStoragePath
	localStorageUrl, _ := httpx.GetURL(conf.Server.Host, conf.Server.Port, conf.MediaResource.LocalStorage.StoragePath)
	if conf.MediaResource.OSS.Enable {
		minioConfig := conf.MediaResource.OSS.Minio
		c, err = minio.New(minioConfig.Endpoint, &minio.Options{
			Creds:  credentials.NewStaticV4(minioConfig.Credentials.AccessKey, minioConfig.Credentials.SecretKey, ""),
			Secure: minioConfig.UseSSL,
		})

		if err != nil {
			panic(errors.Wrap(err, "minio client init failed"))
		}
	} else {
		if conf.MediaResource.LocalStorage.StoragePath != "" {
			localStoragePath = filepath.Join(localStoragePath, conf.MediaResource.LocalStorage.StoragePath)
		}
	}

	return &MediaResourceUseCase{
		db:               db,
		OSSClient:        c,
		LocalStoragePath: localStoragePath,
		LocalStorageUrl:  localStorageUrl,
	}
}

type FindManyMediaResourcesOption struct {
	Ids      []int64
	LikeName string
	Types    []string
	types.PageEmbedOption
}

func (uc *MediaResourceUseCase) buildFindQueryNoPage(db *gorm.DB, opt *FindManyMediaResourcesOption) *gorm.DB {
	if len(opt.Ids) > 0 {
		db = db.Where("id IN ?", opt.Ids)
	}
	if len(opt.Types) > 0 {
		db = db.Where("media_type IN ?", opt.Types)
	}

	if opt.LikeName != "" {
		db = db.Where("filename LIKE ?", "%"+opt.LikeName+"%")
	}

	return db
}

func (uc *MediaResourceUseCase) FindAllMediaResources(ctx context.Context, opt *FindManyMediaResourcesOption) (mediaResources []*media.MediaResource, err error) {
	query := uc.db.WithContext(ctx).Model(&media.MediaResource{})

	query = uc.buildFindQueryNoPage(query, opt)
	if err := query.
		//Debug().
		Find(&mediaResources).Error; err != nil {
		panic(errors.Wrap(err, "find all media resources failed"))
	}
	return mediaResources, err
}

func (uc *MediaResourceUseCase) FindManyMediaResources(ctx context.Context, opt *FindManyMediaResourcesOption) (pageList types.Page[*media.MediaResource], err error) {
	var products []*media.MediaResource
	db := uc.db.WithContext(ctx).Model(&media.MediaResource{})

	db = uc.buildFindQueryNoPage(db, opt)

	var count int64
	if err := db.Count(&count).Error; err != nil {
		panic(err)
	}

	opt.DefaultPageIfNotSet()
	if opt.PageIndex != 0 && opt.PageSize != 0 {
		db.Offset((opt.PageIndex - 1) * opt.PageSize).Limit(opt.PageSize)
	}

	if err := db.
		//Debug().
		Find(&products).Error; err != nil {
		panic(err)
	}

	return types.Page[*media.MediaResource]{
		List:      products,
		PageIndex: opt.PageIndex,
		PageSize:  opt.PageSize,
		Total:     count,
	}, nil
}

func (uc *MediaResourceUseCase) CreateMediaResource(ctx context.Context, resource *media.MediaResource) error {
	if err := uc.db.WithContext(ctx).Create(&resource).Error; err != nil {
		return err
	}
	return nil
}
func (uc *MediaResourceUseCase) CreateMediaResources(ctx context.Context, resources []*media.MediaResource) error {
	if err := uc.db.WithContext(ctx).Create(&resources).Error; err != nil {
		return err
	}
	return nil
}

func (uc *MediaResourceUseCase) MakeProductMediaResource(ctx context.Context, handle *multipart.FileHeader) (resource *media.MediaResource, err error) {
	return uc.MakeMediaResource(ctx, BucketMediaResourceProduct, handle)
}
func (uc *MediaResourceUseCase) MakeMediaResource(ctx context.Context, bucket string, handle *multipart.FileHeader) (resource *media.MediaResource, err error) {

	if uc.OSSClient != nil {
		resource, err = uc.MakeOSSResource(ctx, bucket, handle)
	} else {
		resource, err = uc.MakeLocalResource(ctx, bucket, handle)
	}

	if err != nil {
		return nil, err
	}

	err = uc.CreateMediaResource(ctx, resource)

	return
}

func (uc *MediaResourceUseCase) MakeLocalResource(ctx context.Context, bucket string, handle *multipart.FileHeader) (resource *media.MediaResource, err error) {

	// 获取文件名和文件大小
	filename := handle.Filename
	filesize := handle.Size

	// 模拟将文件保存到本地存储的逻辑
	// 这里可以根据实际需求进行处理，比如将文件保存到指定的文件夹或存储路径中
	// 以下示例将文件保存到名为 "uploads" 的文件夹下
	bucketPath := filepath.Join(uc.LocalStoragePath, bucket)
	// 检查目录是否存在
	if _, err = os.Stat(bucketPath); os.IsNotExist(err) {
		// 目录不存在，创建目录
		err = os.MkdirAll(bucketPath, 0755)
		if err != nil {
			return nil, err
		}
	}

	uploadPath := filepath.Join(bucketPath, filename)
	err = filex.SaveFileToLocal(handle, uploadPath)
	if err != nil {
		return nil, err
	}

	contentType := handle.Header.Get("Content-Type")

	url, err := httpx.AppendURIs(uc.LocalStorageUrl, uploadPath)
	if err != nil {
		return nil, err
	}

	// 构建媒体资源对象
	resource = &media.MediaResource{
		BucketName:   bucket,
		Filename:     filename,
		Size:         filesize,
		Url:          url,
		ContentType:  handle.Header.Get("Content-Type"),
		ResourceType: filex.GetMediaType(contentType),
	}

	return resource, nil
}

func (uc *MediaResourceUseCase) GetOSSResourceURL(bucket string, key string) (string, error) {

	uri, err := uc.GetOSSResourceURI(bucket, key)
	if err != nil {
		return "", nil
	}
	endpoint := uc.OSSClient.EndpointURL()
	url := endpoint.String() + uri

	return url, err
}

func (uc *MediaResourceUseCase) GetOSSResourceURI(bucket string, key string) (string, error) {
	endpoint := uc.OSSClient.EndpointURL()
	url, err := httpx.AppendURIs(endpoint.String(), bucket, key)
	return url, err
}

func (uc *MediaResourceUseCase) MakeOSSResource(ctx context.Context, bucket string, handle *multipart.FileHeader) (resource *media.MediaResource, err error) {

	err = uc.CheckBucketExits(ctx, bucket)
	if err != nil {
		return nil, err
	}

	// Upload the resource file
	objectName := fmt.Sprintf("%d_%s", handle.Size, handle.Filename)
	//objectName := handle.Filename
	filePath, _ := handle.Open()
	contentType := handle.Header.Get("Content-Type")
	info, err := uc.OSSClient.PutObject(ctx, bucket, objectName, filePath, handle.Size, minio.PutObjectOptions{ContentType: contentType})
	//fmt2.Dump(info)

	url, _ := uc.GetOSSResourceURI(bucket, info.Key)

	resource = &media.MediaResource{
		BucketName:   bucket,
		Filename:     info.Key,
		Size:         info.Size,
		Url:          url,
		ContentType:  contentType,
		ResourceType: filex.GetMediaType(contentType),
	}

	return resource, err
}

func (uc *MediaResourceUseCase) MakeOSSResourceByBase64String(ctx context.Context, bucket string, base64Data string) (resource *media.MediaResource, err error) {
	// 解码base64数据
	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return nil, fmt.Errorf("base64数据解码失败：%w", err)
	}

	return uc.MakeOSSResourceByBase64Data(ctx, bucket, data)

}

func (uc *MediaResourceUseCase) GetBase64DataFromMedia(ctx context.Context, media *media.MediaResource) (string, error) {
	// 从OSS中获取图片数据
	url, err := uc.GetOSSResourceURL(media.BucketName, media.Filename)
	if err != nil {
		return "", err
	}

	// 加载图片，转变成base64
	// 通过HTTP GET请求获取图片数据
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("failed to fetch media data")
	}

	// 读取图片数据
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// 将图片数据转换为Base64
	base64Data := base64.StdEncoding.EncodeToString(data)

	return base64Data, err
}

func (uc *MediaResourceUseCase) MakeOSSResourceByBase64Data(ctx context.Context, bucket string, data []byte) (resource *media.MediaResource, err error) {

	err = uc.CheckBucketExits(ctx, bucket)
	if err != nil {
		return nil, err
	}

	// 创建一个新的MinIO对象，并使用随机名称
	objectName := fmt.Sprintf("object_%d", time.Now().UnixNano())

	// 准备对象元数据（可选）
	objectMetadata := make(map[string]string)
	objectMetadata["Content-Type"] = "image/png" // 替换为实际内容类型
	//objectMetadata["Content-Type"] = "image/jpeg" // 替换为实际内容类型
	//objectMetadata["Content-Type"] = "image/webp" // 替换为实际内容类型

	// 上传对象到MinIO
	contentType := objectMetadata["Content-Type"]
	info, err := uc.OSSClient.PutObject(ctx, bucket, objectName, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return nil, fmt.Errorf("上传对象到MinIO失败：%w", err)
	}

	// 如果需要，你现在可以创建并返回你的MediaResource结构
	url := fmt.Sprintf("%s/%s", bucket, objectName)
	mediaResource := &media.MediaResource{

		BucketName:   bucket,
		Filename:     info.Key,
		Size:         info.Size,
		Url:          url,
		ContentType:  contentType,
		ResourceType: filex.GetMediaType(contentType),
	}

	return mediaResource, nil
}

func (uc *MediaResourceUseCase) CheckBucketExits(ctx context.Context, bucket string) error {

	exist, err := uc.OSSClient.BucketExists(ctx, bucket)
	if err != nil {
		return err
	}

	if !exist {
		location := "us-east-1"
		err = uc.OSSClient.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: location})
		if err != nil {
			return err
		}
		// 设置存储桶策略
		policy := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": "*",
				"Action": ["s3:GetObject"],
				"Resource": ["arn:aws:s3:::` + bucket + `/*"]
			}
		]
	}`
		err = uc.OSSClient.SetBucketPolicy(context.Background(), bucket, policy)
		if err != nil {
			return err
		}
	}

	return nil
}

func (uc *MediaResourceUseCase) GetCachedBase64DataFromMedia(ctx context.Context, cache cache.CacheInterface, media *media.MediaResource) (string, error) {
	// 从缓存中获取图片数据
	data, err := cache.Remember(media.Filename, 3600*time.Hour, func() (interface{}, error) {
		// 将url图片转换成Base64
		//fmt2.Dump("get data from oss", media.Filename)
		return uc.GetBase64DataFromMedia(ctx, media)

	})
	if err != nil {
		return "", err
	}

	return data.(string), err
}

func (uc *MediaResourceUseCase) UploadVideoFromURL(ctx context.Context, mediaConfig *config.MediaResource, bucket string, videoURL string) error {
	minioConfig := mediaConfig.OSS.Minio
	client, err := minio.NewCore(minioConfig.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(minioConfig.Credentials.AccessKey, minioConfig.Credentials.SecretKey, ""),
		Secure: minioConfig.UseSSL,
	})

	// 从视频 URL 下载视频文件
	resp, err := http.Get(videoURL)
	if err != nil {
		return errors.Wrap(err, "failed to download video from URL")
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("failed to download video, status code: %d", resp.StatusCode)
	}

	// 设置上传的对象名称
	objectName := "uploaded_video.mp4" // 根据需要设置对象名称

	// 创建分片上传
	uploadID, err := client.NewMultipartUpload(ctx, bucket, objectName, minio.PutObjectOptions{
		ContentType: "video/mp4", // 根据实际视频类型设置
	})
	if err != nil {
		return errors.Wrap(err, "failed to initiate multipart upload")
	}

	// 定义分片大小，MinIO 的限制通常为 5MB
	const partSize = 5 * 1024 * 1024 // 5MB
	var partNumber int
	var completeParts []minio.CompletePart // 存储每个分片的信息

	// 循环读取分片并上传
	for {
		buf := make([]byte, partSize)
		n, err := resp.Body.Read(buf)
		if err == io.EOF {
			break // 到达文件结尾
		}
		if err != nil {
			return errors.Wrap(err, "failed to read from response body")
		}

		// 上传分片
		partNumber++
		partReader := io.LimitReader(resp.Body, int64(n)) // 限制读取的字节数
		objectPart, err := client.PutObjectPart(ctx, bucket, objectName, uploadID, partNumber, partReader, int64(n), minio.PutObjectPartOptions{})
		if err != nil {
			return errors.Wrap(err, "failed to upload part")
		}

		// 将上传的分片信息添加到 completeParts 列表中
		completeParts = append(completeParts, minio.CompletePart{
			ETag:       objectPart.ETag, // 使用 ObjectPart 中的 ETag
			PartNumber: partNumber,      // 使用分片编号
		})
	}

	// 完成分片上传
	_, err = client.CompleteMultipartUpload(ctx, bucket, objectName, uploadID, completeParts, minio.PutObjectOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to complete multipart upload")
	}

	fmt.Println("Upload completed successfully")
	return nil
}
