package media

// StorageOptions 定义媒体存储配置，包含默认驱动和各驱动子配置。
type StorageOptions struct {
	RequireExistingLocalPath bool // 仅供维护扫描，初始化时不创建本地目录
	DefaultDriver            string
	TTLSeconds               int32
	ImageDerivatives         ImageDerivativeOptions
	Local                    StorageLocalOptions
	S3                       StorageS3Options
}

// ImageDerivativeOptions 只限制派生处理，不改变原稿上传、存储和下载。
// MaxWorkingBytes 是每个处理任务的保守内存估算预算，进程并发由 MaxParallelJobs 限制。
type ImageDerivativeOptions struct {
	MaxSourceBytes  int64
	MaxPixels       int64
	MaxWorkingBytes int64
	MaxParallelJobs int
}

// StorageLocalOptions 为本地文件系统驱动的配置。
type StorageLocalOptions struct {
	BasePath           string
	PublicBaseURL      string
	UploadTokenSecret  string
	PublicTokenSecret  string
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
