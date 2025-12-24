package media

// StorageOptions 定义媒体存储配置，包含默认驱动和各驱动子配置。
type StorageOptions struct {
	DefaultDriver string
	TTLSeconds    int32
	Local         StorageLocalOptions
	S3            StorageS3Options
}

// StorageLocalOptions 为本地文件系统驱动的配置。
type StorageLocalOptions struct {
	BasePath           string
	PublicBaseURL      string
	UploadTokenSecret  string
	MaxUploadSizeBytes int64
}

// StorageS3Options 为 S3 兼容驱动的配置。
type StorageS3Options struct {
	Endpoint        string
	Region          string
	AccessKey       string
	SecretKey       string
	SessionToken    string
	Bucket          string
	UseSSL          bool
	ForcePathStyle  bool
	ExternalDomain  string
	PresignEndpoint string
}
