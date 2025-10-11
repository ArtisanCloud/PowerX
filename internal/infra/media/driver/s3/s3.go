package s3

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
)

// Options 定义 S3 驱动初始化所需参数。
type Options struct {
	Name            string
	Endpoint        string
	AccessKey       string
	SecretKey       string
	SessionToken    string
	Region          string
	UseSSL          bool
	ForcePathStyle  bool
	Bucket          string
	ExternalDomain  string
	PresignEndpoint string
	DefaultTTL      time.Duration
}

// Driver 基于 MinIO SDK 的 S3 驱动实现。
type Driver struct {
	name            string
	bucket          string
	externalDomain  string
	presignEndpoint string
	defaultTTL      time.Duration
	client          *minio.Client
}

const (
	opPut     = "put"
	opGet     = "get"
	opDelete  = "delete"
	opPresign = "presign"

	maxPresignTTL = 7 * 24 * time.Hour
)

// New 创建 S3 驱动。
func New(opts Options) (*Driver, error) {
	if strings.TrimSpace(opts.Endpoint) == "" {
		return nil, fmt.Errorf("s3 driver: endpoint 不能为空")
	}
	if strings.TrimSpace(opts.Bucket) == "" {
		return nil, fmt.Errorf("s3 driver: bucket 不能为空")
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "s3"
	}
	client, err := minio.New(opts.Endpoint, &minio.Options{
		AccessKey:      opts.AccessKey,
		SecretKey:      opts.SecretKey,
		SessionToken:   opts.SessionToken,
		Secure:         opts.UseSSL,
		Region:         opts.Region,
		ForcePathStyle: opts.ForcePathStyle,
	})
	if err != nil {
		return nil, fmt.Errorf("s3 driver: 创建客户端失败: %w", err)
	}

	ttl := opts.DefaultTTL
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	if ttl > maxPresignTTL {
		ttl = maxPresignTTL
	}

	driver := &Driver{
		name:            name,
		bucket:          opts.Bucket,
		externalDomain:  strings.TrimSpace(opts.ExternalDomain),
		presignEndpoint: strings.TrimSpace(opts.PresignEndpoint),
		defaultTTL:      ttl,
		client:          client,
	}
	return driver, nil
}

// Name 返回驱动名称。
func (d *Driver) Name() string {
	return d.name
}

func (d *Driver) resolveBucket(inBucket string) string {
	if strings.TrimSpace(inBucket) != "" {
		return strings.TrimSpace(inBucket)
	}
	return d.bucket
}

func sanitizeKey(key string) (string, error) {
	trimmed := strings.TrimLeft(key, "/")
	if trimmed == "" {
		return "", driver.ErrInvalidArgument
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") {
		return "", driver.ErrInvalidArgument
	}
	return cleaned, nil
}

// Put 上传对象到 S3。
func (d *Driver) Put(ctx context.Context, in driver.PutObjectInput) (*driver.PutObjectResult, error) {
	key, err := sanitizeKey(in.ObjectKey)
	if err != nil {
		return nil, err
	}
	bucket := d.resolveBucket(in.Bucket)
	if in.Body == nil {
		return nil, driver.ErrInvalidArgument
	}

	opts := minio.PutObjectOptions{ContentType: in.ContentType}
	if len(in.Metadata) > 0 {
		opts.UserMetadata = make(map[string]string, len(in.Metadata))
		for k, v := range in.Metadata {
			opts.UserMetadata[k] = v
		}
	}
	if !in.Overwrite {
		opts.OnlyIfAbsent = true
	}

	info, err := d.client.PutObject(ctx, bucket, key, in.Body, in.Size, opts)
	if err != nil {
		if isNotFoundErr(err) {
			return nil, driver.ErrNotFound
		}
		if isConflictErr(err) {
			return nil, driver.ErrConflict
		}
		return nil, driver.WrapError(d.name, opPut, err)
	}
	return &driver.PutObjectResult{
		Bucket:    bucket,
		ObjectKey: key,
		Size:      info.Size,
		ETag:      info.ETag,
		VersionID: info.VersionID,
	}, nil
}

// Get 下载对象内容。
func (d *Driver) Get(ctx context.Context, in driver.GetObjectInput) (*driver.GetObjectResult, error) {
	key, err := sanitizeKey(in.ObjectKey)
	if err != nil {
		return nil, err
	}
	bucket := d.resolveBucket(in.Bucket)
	obj, err := d.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		if isNotFoundErr(err) {
			return nil, driver.ErrNotFound
		}
		return nil, driver.WrapError(d.name, opGet, err)
	}

	stat, statErr := obj.Stat()
	if statErr != nil {
		obj.Close()
		if isNotFoundErr(statErr) {
			return nil, driver.ErrNotFound
		}
		return nil, driver.WrapError(d.name, opGet, statErr)
	}

	return &driver.GetObjectResult{
		Bucket:       bucket,
		ObjectKey:    key,
		Body:         obj,
		Size:         stat.Size,
		ContentType:  stat.ContentType,
		LastModified: stat.LastModified.UTC(),
		ETag:         stat.ETag,
	}, nil
}

// Delete 删除对象。
func (d *Driver) Delete(ctx context.Context, in driver.DeleteObjectInput) error {
	key, err := sanitizeKey(in.ObjectKey)
	if err != nil {
		return err
	}
	bucket := d.resolveBucket(in.Bucket)

	opts := minio.RemoveObjectOptions{ForceDelete: in.Force}
	if in.VersionID != "" {
		opts.GovernanceBypass = true
		opts.VersionID = in.VersionID
	}
	err = d.client.RemoveObject(ctx, bucket, key, opts)
	if err != nil {
		if isNotFoundErr(err) {
			if in.Force {
				return nil
			}
			return driver.ErrNotFound
		}
		return driver.WrapError(d.name, opDelete, err)
	}
	return nil
}

// GenerateURL 生成预签名 URL，支持 GET/PUT。
func (d *Driver) GenerateURL(ctx context.Context, in driver.GenerateURLInput) (*driver.GenerateURLOutput, error) {
	key, err := sanitizeKey(in.ObjectKey)
	if err != nil {
		return nil, err
	}
	bucket := d.resolveBucket(in.Bucket)

	ttl := in.TTL
	if ttl <= 0 {
		ttl = d.defaultTTL
	}
	if ttl <= 0 {
		return nil, driver.ErrInvalidArgument
	}
	if ttl > maxPresignTTL {
		return nil, driver.ErrInvalidArgument
	}

	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		method = http.MethodGet
	}

	reqHeader := make(http.Header)
	for k, values := range in.Headers {
		for _, v := range values {
			reqHeader.Add(k, v)
		}
	}

	var urlStr string
	var presignErr error
	expireAt := time.Now().Add(ttl)
	switch method {
	case http.MethodGet:
		params := make(url.Values)
		if ct := reqHeader.Get("response-content-type"); ct != "" {
			params.Set("response-content-type", ct)
		}
		if disp := reqHeader.Get("response-content-disposition"); disp != "" {
			params.Set("response-content-disposition", disp)
		}
		urlObj, err := d.client.PresignedGetObject(ctx, bucket, key, ttl, params)
		if err != nil {
			presignErr = err
			break
		}
		urlStr = d.rewriteURL(urlObj)
	case http.MethodPut:
		urlObj, err := d.client.PresignedPutObject(ctx, bucket, key, ttl)
		if err != nil {
			presignErr = err
			break
		}
		urlStr = d.rewriteURL(urlObj)
		if in.ContentType != "" {
			reqHeader.Set("Content-Type", in.ContentType)
		}
	default:
		return nil, driver.ErrUnsupported
	}

	if presignErr != nil {
		if isNotFoundErr(presignErr) {
			return nil, driver.ErrNotFound
		}
		return nil, driver.WrapError(d.name, opPresign, presignErr)
	}

	return &driver.GenerateURLOutput{
		Bucket:    bucket,
		ObjectKey: key,
		Method:    method,
		URL:       urlStr,
		ExpireAt:  expireAt,
		Headers:   reqHeader,
	}, nil
}

func (d *Driver) rewriteURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	out := *u
	if d.presignEndpoint != "" {
		if replacement, err := url.Parse(d.presignEndpoint); err == nil {
			if replacement.Scheme != "" {
				out.Scheme = replacement.Scheme
			}
			if replacement.Host != "" {
				out.Host = replacement.Host
			}
			if strings.Trim(replacement.Path, "/") != "" {
				out.Path = path.Join(strings.TrimRight(replacement.Path, "/"), strings.TrimLeft(out.Path, "/"))
			}
		}
	} else if d.externalDomain != "" {
		out.Host = d.externalDomain
	}
	return out.String()
}

// HealthCheck 校验桶是否存在、客户端可用。
func (d *Driver) HealthCheck(ctx context.Context) error {
	exists, err := d.client.BucketExists(ctx, d.bucket)
	if err != nil {
		return driver.WrapError(d.name, "health", err)
	}
	if !exists {
		return driver.WrapError(d.name, "health", fmt.Errorf("bucket %s 不存在", d.bucket))
	}
	return nil
}

func isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	resp := minio.ToErrorResponse(err)
	switch resp.Code {
	case "NoSuchKey", "NoSuchBucket", "NotFound", "NoSuchUpload", "NoSuchVersion":
		return true
	}
	return false
}

func isConflictErr(err error) bool {
	if err == nil {
		return false
	}
	resp := minio.ToErrorResponse(err)
	switch resp.Code {
	case "PreconditionFailed", "EntityAlreadyExists", "BucketAlreadyOwnedByYou", "BucketAlreadyExists":
		return true
	}
	return false
}
