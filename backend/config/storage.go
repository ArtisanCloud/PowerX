package config

// StorageConfig 定义媒体驱动配置（local 与 s3/MinIO）。
type StorageConfig struct {
	DefaultDriver    string                `yaml:"default_driver"`
	TTLSeconds       int32                 `yaml:"ttl_seconds"`
	ImageDerivatives ImageDerivativeConfig `yaml:"image_derivatives"`
	Local            LocalStorageConfig    `yaml:"local"`
	S3               S3StorageConfig       `yaml:"s3"`
}

// ImageDerivativeConfig 仅用于缩略图／预览的有界处理，原稿不受派生像素限制。
// MaxWorkingBytes 是每任务估算预算，总并发预算由 MaxParallelJobs 共同决定。
type ImageDerivativeConfig struct {
	MaxSourceBytes  int64 `yaml:"max_source_bytes"`
	MaxPixels       int64 `yaml:"max_pixels"`
	MaxWorkingBytes int64 `yaml:"max_working_bytes"`
	MaxParallelJobs int   `yaml:"max_parallel_jobs"`
}

// LocalStorageConfig 为本地驱动提供路径配置。
type LocalStorageConfig struct {
	BasePath          string `yaml:"base_path"`
	PublicBaseURL     string `yaml:"public_base_url"`
	UploadTokenSecret string `yaml:"upload_token_secret"`
	// PublicTokenSecret 用于公开资源入口（/media/:uuid/resource）的临时访问 token（HMAC）签名与校验。
	// 为空时：公开入口仅允许 published 资源访问；非 published 资源无法生成可直接打开的 presign(download) 链接。
	PublicTokenSecret  string `yaml:"public_token_secret"`
	MaxUploadSizeBytes int64  `yaml:"max_upload_size_bytes"`
}

// S3StorageConfig 兼容 AWS S3 与 MinIO。
type S3StorageConfig struct {
	Endpoint        string `yaml:"endpoint"`
	Region          string `yaml:"region"`
	AccessKey       string `yaml:"access_key"`
	SecretKey       string `yaml:"secret_key"`
	SessionToken    string `yaml:"session_token"`
	Bucket          string `yaml:"bucket"`
	UseSSL          bool   `yaml:"use_ssl"`
	ForcePathStyle  bool   `yaml:"force_path_style"`
	ExternalDomain  string `yaml:"external_domain"`
	PresignEndpoint string `yaml:"presign_endpoint"`
}
