package config

type MediaResource struct {
	LocalStorage struct {
		StoragePath string
	}
	OSS struct {
		Enable bool
		Minio  struct {
			Endpoint    string
			Credentials struct {
				AccessKey string
				SecretKey string
			}
			UseSSL bool
		}
	}
}
