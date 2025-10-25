package driver

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

// 统一的驱动级错误定义，方便 Service 层做细粒度处理。
var (
	ErrInvalidArgument = errors.New("media: invalid argument")
	ErrNotFound        = errors.New("media: object not found")
	ErrPermission      = errors.New("media: permission denied")
	ErrConflict        = errors.New("media: conflict")
	ErrUnsupported     = errors.New("media: unsupported operation")
)

// Error 用于包装底层驱动返回的错误并携带操作信息，便于日志与上层诊断。
type Error struct {
	Driver string
	Op     string
	Err    error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	return "media driver " + e.Driver + " " + e.Op + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// WrapError 构造带上下文的驱动错误。
func WrapError(driver, op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrInvalidArgument) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrPermission) || errors.Is(err, ErrConflict) || errors.Is(err, ErrUnsupported) {
		return err
	}
	return &Error{Driver: driver, Op: op, Err: err}
}

// PutObjectInput 定义写入对象时所需的参数。
type PutObjectInput struct {
	Bucket      string
	ObjectKey   string
	Body        io.Reader
	Size        int64
	ContentType string
	Metadata    map[string]string
	Overwrite   bool
}

// PutObjectResult 返回对象写入后的元信息。
type PutObjectResult struct {
	Bucket    string
	ObjectKey string
	Size      int64
	ETag      string
	VersionID string
}

// GetObjectInput 定义读取对象时的参数。
type GetObjectInput struct {
	Bucket    string
	ObjectKey string
}

// GetObjectResult 封装对象读取返回值，包含数据流与元信息。
type GetObjectResult struct {
	Bucket       string
	ObjectKey    string
	Body         io.ReadCloser
	Size         int64
	ContentType  string
	LastModified time.Time
	ETag         string
}

// DeleteObjectInput 定义删除对象的参数。
type DeleteObjectInput struct {
	Bucket    string
	ObjectKey string
	VersionID string
	Force     bool
}

// GenerateURLInput 用于生成访问 URL（预签名/公共链接）。
type GenerateURLInput struct {
	Bucket      string
	ObjectKey   string
	Method      string
	TTL         time.Duration
	ContentType string
	Headers     http.Header
}

// GenerateURLOutput 为 URL 生成结果。
type GenerateURLOutput struct {
	Bucket    string
	ObjectKey string
	Method    string
	URL       string
	ExpireAt  time.Time
	Headers   http.Header
}

// StorageDriver 统一定义媒体对象驱动应实现的能力。
type StorageDriver interface {
	Name() string

	Put(ctx context.Context, in PutObjectInput) (*PutObjectResult, error)
	Get(ctx context.Context, in GetObjectInput) (*GetObjectResult, error)
	Delete(ctx context.Context, in DeleteObjectInput) error
	GenerateURL(ctx context.Context, in GenerateURLInput) (*GenerateURLOutput, error)

	HealthCheck(ctx context.Context) error
}
