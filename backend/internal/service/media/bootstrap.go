package media

import (
	"context"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver/local"
	s3driver "github.com/ArtisanCloud/PowerX/internal/infra/media/driver/s3"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
)

// BuildMediaStack 根据存储配置初始化媒体驱动管理器与媒体服务。
func BuildMediaStack(ctx context.Context, db *gorm.DB, audit auditsvc.Service, opts StorageOptions) (*mediamgr.MediaManager, *MediaService) {
	if ctx == nil {
		ctx = context.Background()
	}

	manager := mediamgr.New(opts.DefaultDriver)

	if drv, err := local.New(local.Options{
		Name:          "local",
		BasePath:      opts.Local.BasePath,
		PublicBaseURL: opts.Local.PublicBaseURL,
		EnableUpload:  true,
		UploadToken:   opts.Local.UploadTokenSecret,
		MaxUploadSize: opts.Local.MaxUploadSizeBytes,
	}); err != nil {
		pxlog.Warn(ctx, "init local media driver failed: "+err.Error())
	} else {
		manager.RegisterDriver(drv)
	}

	if strings.TrimSpace(opts.S3.Endpoint) != "" {
		if drv, err := s3driver.New(s3driver.Options{
			Name:            "s3",
			Endpoint:        opts.S3.Endpoint,
			AccessKey:       opts.S3.AccessKey,
			SecretKey:       opts.S3.SecretKey,
			SessionToken:    opts.S3.SessionToken,
			Region:          opts.S3.Region,
			UseSSL:          opts.S3.UseSSL,
			ForcePathStyle:  opts.S3.ForcePathStyle,
			Bucket:          opts.S3.Bucket,
			ExternalDomain:  opts.S3.ExternalDomain,
			PresignEndpoint: opts.S3.PresignEndpoint,
			DefaultTTL:      time.Duration(opts.TTLSeconds) * time.Second,
		}); err != nil {
			pxlog.Warn(ctx, "init s3 media driver failed: "+err.Error())
		} else {
			manager.RegisterDriver(drv)
		}
	}

	if def := strings.TrimSpace(opts.DefaultDriver); def != "" {
		if err := manager.SetDefaultDriver(def); err != nil {
			pxlog.Warn(ctx, "set default media driver failed: "+err.Error())
		}
	}

	ttl := time.Duration(opts.TTLSeconds) * time.Second
	svc := NewMediaService(db, nil, manager, audit, ttl)
	svc.SetPublicResourceTokenSecret(opts.Local.PublicTokenSecret)

	return manager, svc
}
