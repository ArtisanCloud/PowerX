package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
)

// Options 定义本地驱动的初始化参数。
type Options struct {
	Name          string
	BasePath      string
	PublicBaseURL string
	DirPerm       os.FileMode
	FilePerm      os.FileMode
}

// Driver 为本地文件系统驱动实现。
type Driver struct {
	name          string
	basePath      string
	publicBaseURL string
	dirPerm       os.FileMode
	filePerm      os.FileMode
}

// New 根据配置创建本地驱动实例，并确保基础目录存在。
func New(opts Options) (*Driver, error) {
	base := strings.TrimSpace(opts.BasePath)
	if base == "" {
		return nil, fmt.Errorf("local driver: base path 不能为空")
	}
	if opts.DirPerm == 0 {
		opts.DirPerm = 0o755
	}
	if opts.FilePerm == 0 {
		opts.FilePerm = 0o644
	}
	if err := os.MkdirAll(base, opts.DirPerm); err != nil {
		return nil, fmt.Errorf("local driver: 初始化目录失败: %w", err)
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "local"
	}
	var publicURL string
	if strings.TrimSpace(opts.PublicBaseURL) != "" {
		u, err := url.Parse(opts.PublicBaseURL)
		if err != nil {
			return nil, fmt.Errorf("local driver: public base url 非法: %w", err)
		}
		publicURL = strings.TrimRight(u.String(), "/")
	}

	return &Driver{
		name:          name,
		basePath:      base,
		publicBaseURL: publicURL,
		dirPerm:       opts.DirPerm,
		filePerm:      opts.FilePerm,
	}, nil
}

// Name 返回驱动名称。
func (d *Driver) Name() string {
	return d.name
}

func (d *Driver) resolvePath(bucket, key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", driver.ErrInvalidArgument
	}
	sanitizedKey := filepath.Clean(strings.TrimLeft(key, "/"))
	if sanitizedKey == "." || sanitizedKey == ".." {
		return "", driver.ErrInvalidArgument
	}
	if strings.HasPrefix(sanitizedKey, "../") || strings.HasPrefix(sanitizedKey, "..\\") {
		return "", driver.ErrInvalidArgument
	}

	parts := []string{d.basePath}
	if bucket = strings.TrimSpace(bucket); bucket != "" {
		cleanedBucket := filepath.Clean(bucket)
		if cleanedBucket == "." || cleanedBucket == ".." || strings.HasPrefix(cleanedBucket, "../") || strings.HasPrefix(cleanedBucket, "..\\") {
			return "", driver.ErrInvalidArgument
		}
		parts = append(parts, cleanedBucket)
	}
	fullPath := filepath.Join(append(parts, sanitizedKey)...)
	rel, err := filepath.Rel(d.basePath, fullPath)
	if err != nil {
		return "", driver.WrapError(d.name, "resolve_path", err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", driver.ErrInvalidArgument
	}
	return fullPath, nil
}

func (d *Driver) relativeObjectPath(bucket, key string) (string, error) {
	fullPath, err := d.resolvePath(bucket, key)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(d.basePath, fullPath)
	if err != nil {
		return "", driver.WrapError(d.name, "relative", err)
	}
	if strings.HasPrefix(rel, "..") {
		return "", driver.ErrInvalidArgument
	}
	return filepath.ToSlash(rel), nil
}

// Put 写入文件到本地文件系统。
func (d *Driver) Put(ctx context.Context, in driver.PutObjectInput) (*driver.PutObjectResult, error) {
	if in.Body == nil {
		return nil, driver.ErrInvalidArgument
	}
	path, err := d.resolvePath(in.Bucket, in.ObjectKey)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(path), d.dirPerm); err != nil {
		return nil, driver.WrapError(d.name, "mkdir", err)
	}

	flags := os.O_CREATE | os.O_WRONLY
	if in.Overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, d.filePerm)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, driver.ErrConflict
		}
		return nil, driver.WrapError(d.name, "open", err)
	}
	defer file.Close()

	written, err := io.Copy(file, in.Body)
	if err != nil {
		return nil, driver.WrapError(d.name, "write", err)
	}
	if in.Size > 0 && written != in.Size {
		return nil, driver.WrapError(d.name, "write", fmt.Errorf("写入长度不符: expect=%d actual=%d", in.Size, written))
	}

	info, err := file.Stat()
	if err != nil {
		return nil, driver.WrapError(d.name, "stat", err)
	}

	return &driver.PutObjectResult{
		Bucket:    in.Bucket,
		ObjectKey: in.ObjectKey,
		Size:      info.Size(),
	}, nil
}

// Get 打开文件并返回读取句柄。
func (d *Driver) Get(ctx context.Context, in driver.GetObjectInput) (*driver.GetObjectResult, error) {
	path, err := d.resolvePath(in.Bucket, in.ObjectKey)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, driver.ErrNotFound
		}
		return nil, driver.WrapError(d.name, "open", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, driver.WrapError(d.name, "stat", err)
	}

	// 尝试检测 MIME 类型（非关键路径，失败则忽略）。
	var contentType string
	header := make([]byte, 512)
	n, _ := file.Read(header)
	if n > 0 {
		contentType = http.DetectContentType(header[:n])
		file.Seek(0, io.SeekStart)
	}

	return &driver.GetObjectResult{
		Bucket:       in.Bucket,
		ObjectKey:    in.ObjectKey,
		Body:         file,
		Size:         info.Size(),
		ContentType:  contentType,
		LastModified: info.ModTime().UTC(),
	}, nil
}

// Delete 删除指定文件。
func (d *Driver) Delete(ctx context.Context, in driver.DeleteObjectInput) error {
	path, err := d.resolvePath(in.Bucket, in.ObjectKey)
	if err != nil {
		return err
	}
	if err = os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if in.Force {
				return nil
			}
			return driver.ErrNotFound
		}
		return driver.WrapError(d.name, "delete", err)
	}
	return nil
}

// GenerateURL 返回本地静态访问地址，仅支持 GET。
func (d *Driver) GenerateURL(ctx context.Context, in driver.GenerateURLInput) (*driver.GenerateURLOutput, error) {
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet {
		return nil, driver.ErrUnsupported
	}
	if d.publicBaseURL == "" {
		return nil, driver.WrapError(d.name, "generate_url", errors.New("未配置 public_base_url"))
	}
	rel, err := d.relativeObjectPath(in.Bucket, in.ObjectKey)
	if err != nil {
		return nil, err
	}
	expireAt := time.Now().Add(in.TTL)
	if in.TTL <= 0 {
		expireAt = time.Now().Add(12 * time.Hour)
	}
	urlStr := d.publicBaseURL
	if rel != "" {
		urlStr += "/" + strings.ReplaceAll(rel, "\\", "/")
	}
	return &driver.GenerateURLOutput{
		Bucket:    in.Bucket,
		ObjectKey: in.ObjectKey,
		Method:    method,
		URL:       urlStr,
		ExpireAt:  expireAt,
		Headers:   http.Header{},
	}, nil
}

// HealthCheck 确保基础目录存在且可写。
func (d *Driver) HealthCheck(ctx context.Context) error {
	info, err := os.Stat(d.basePath)
	if err != nil {
		return driver.WrapError(d.name, "health", err)
	}
	if !info.IsDir() {
		return driver.WrapError(d.name, "health", fmt.Errorf("%s 不是目录", d.basePath))
	}
	testFile := filepath.Join(d.basePath, ".healthcheck")
	if err = os.WriteFile(testFile, []byte(time.Now().Format(time.RFC3339Nano)), d.filePerm); err != nil {
		return driver.WrapError(d.name, "health", err)
	}
	_ = os.Remove(testFile)
	return nil
}
