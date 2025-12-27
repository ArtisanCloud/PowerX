package config

// StorageConfig 定义媒体驱动配置（local 与 s3/MinIO）。
type StorageConfig struct {
	DefaultDriver string             `yaml:"default_driver"`
	TTLSeconds    int32              `yaml:"ttl_seconds"`
	Local         LocalStorageConfig `yaml:"local"`
	S3            S3StorageConfig    `yaml:"s3"`
}

// LocalStorageConfig 为本地驱动提供路径配置。
type LocalStorageConfig struct {
	BasePath           string `yaml:"base_path"`
	PublicBaseURL      string `yaml:"public_base_url"`
	UploadTokenSecret  string `yaml:"upload_token_secret"`
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
