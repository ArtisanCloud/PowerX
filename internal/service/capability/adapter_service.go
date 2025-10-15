package capability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	capb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/v1"
	"github.com/ArtisanCloud/PowerX/internal/contract/capability"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var (
	// ErrAdapterNotFound 当未注册指定协议的传输适配器时返回。
	ErrAdapterNotFound = errors.New("capability: transport adapter not registered")
	// ErrNoHealthyAdapter 在所有传输通道均不可用时返回。
	ErrNoHealthyAdapter = errors.New("capability: no healthy transport adapter available")
)

// TransportAdapter 统一抽象，所有协议实现必须满足。
type TransportAdapter interface {
	Invoke(ctx context.Context, req *TransportRequest) (*TransportResponse, error)
	Stream(ctx context.Context, req *TransportRequest, sink chan<- *StreamChunk) error
	HealthCheck(ctx context.Context, capabilityKey string) (*TransportHealthReport, error)
	Close(ctx context.Context) error
}

// AdapterMetricsRecorder 用于记录调用指标，默认实现为空操作。
type AdapterMetricsRecorder interface {
	ObserveInvocation(ctx context.Context, transport string, capabilityKey string, duration time.Duration, attempt int, err error)
	ObserveStream(ctx context.Context, transport string, capabilityKey string, err error)
	RecordHealth(ctx context.Context, transport string, capabilityKey string, status string, err error)
}

// AdapterTracer 用于埋点 tracing span。
type AdapterTracer interface {
	StartSpan(ctx context.Context, name string, attributes map[string]string) (context.Context, Span)
}

// Span 表示一次 Trace Span。
type Span interface {
	End(err error)
}

// RetryableError 定义可判定是否可重试的错误。
type RetryableError interface {
	error
	Retryable() bool
}

// TransportRequest 统一的调用请求结构。
type TransportRequest struct {
	RequestID      string
	TraceID        string
	TenantID       uint64
	ActorID        string
	CapabilityKey  string
	Version        string
	Transport      capb.TransportKind
	Payload        map[string]any
	Deadline       time.Time
	RetryContext   *RetryContext
	Metadata       map[string]string
	Stream         *StreamDescriptor
	ToolGrantToken string
	Observability  *ObservabilityContext
}

// ObservabilityContext 传递额外的观测信息。
type ObservabilityContext struct {
	SpanName   string
	Attributes map[string]string
}

// TransportResponse 统一的调用响应结构。
type TransportResponse struct {
	RequestID       string
	Status          ResponseStatus
	Output          map[string]any
	Error           *CapabilityError
	ObservedVersion string
	Metrics         map[string]float64
	Attempt         int
}

// ResponseStatus 统一的响应状态。
type ResponseStatus string

const (
	// ResponseStatusSuccess 代表调用成功。
	ResponseStatusSuccess ResponseStatus = "success"
	// ResponseStatusError 代表调用失败。
	ResponseStatusError ResponseStatus = "error"
)

// StreamChunk 代表流式调用的单个分片。
type StreamChunk struct {
	RequestID string
	Sequence  uint64
	Kind      StreamChunkKind
	Payload   map[string]any
	Timestamp time.Time
	Error     *CapabilityError
}

// StreamChunkKind 表示流式分片类型。
type StreamChunkKind string

const (
	// StreamChunkKindData 流式数据分片。
	StreamChunkKindData StreamChunkKind = "data"
	// StreamChunkKindLog 日志分片。
	StreamChunkKindLog StreamChunkKind = "log"
	// StreamChunkKindEvent 事件分片。
	StreamChunkKindEvent StreamChunkKind = "event"
	// StreamChunkKindError 错误分片。
	StreamChunkKindError StreamChunkKind = "error"
)

// StreamDescriptor 描述流式调用设置。
type StreamDescriptor struct {
	Enabled        bool
	BufferSize     int
	InitialRequest map[string]any
}

// CapabilityError 统一错误模型。
type CapabilityError struct {
	Namespace       string            `json:"namespace,omitempty"`
	Category        string            `json:"category,omitempty"`
	Code            string            `json:"code,omitempty"`
	Severity        string            `json:"severity,omitempty"`
	Stage           string            `json:"stage,omitempty"`
	Message         string            `json:"message,omitempty"`
	SuggestedAction string            `json:"suggested_action,omitempty"`
	Cause           error             `json:"-"`
	Telemetry       map[string]string `json:"telemetry,omitempty"`
	Retryable       bool              `json:"retryable,omitempty"`
}

// Error 实现 error 接口。
func (e *CapabilityError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return fmt.Sprintf("%s.%s.%s", e.Namespace, e.Category, e.Code)
}

// TransportHealthReport 健康检查结果。
type TransportHealthReport struct {
	Status    string
	CheckedAt time.Time
	LastError *CapabilityError
	Metadata  map[string]string
}

// TransportProfile 对外暴露的传输配置视图。
type TransportProfile struct {
	Transport        string                 `json:"transport"`
	Mode             string                 `json:"mode"`
	TimeoutMillis    int                    `json:"timeout_ms"`
	Streaming        bool                   `json:"streaming"`
	Retry            map[string]interface{} `json:"retry,omitempty"`
	QoS              map[string]interface{} `json:"qos,omitempty"`
	EndpointSelector map[string]interface{} `json:"endpoint_selector,omitempty"`
	HealthReport     *TransportHealthReport `json:"health_report,omitempty"`
}

// RetryContext 描述重试策略。
type RetryContext struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	Multiplier     float64
	Idempotent     bool
	ShouldRetry    func(error) bool
}

// AdapterService 管理传输适配器及其持久化配置。
type AdapterService struct {
	db            *gorm.DB
	contractRepo  *caprepo.ContractRepository
	transportRepo *caprepo.TransportProfileRepository

	mu       sync.RWMutex
	adapters map[capb.TransportKind]TransportAdapter
	metrics  AdapterMetricsRecorder
	tracer   AdapterTracer
}

// NewAdapterService 创建 AdapterService 实例。
func NewAdapterService(db *gorm.DB, metrics AdapterMetricsRecorder, tracer AdapterTracer) *AdapterService {
	if metrics == nil {
		metrics = &noopMetricsRecorder{}
	}
	if tracer == nil {
		tracer = noopTracer{}
	}
	return &AdapterService{
		db:            db,
		contractRepo:  caprepo.NewContractRepository(db),
		transportRepo: caprepo.NewTransportProfileRepository(db),
		adapters:      make(map[capb.TransportKind]TransportAdapter),
		metrics:       metrics,
		tracer:        tracer,
	}
}

// RegisterAdapter 注册协议适配器。
func (s *AdapterService) RegisterAdapter(kind capb.TransportKind, adapter TransportAdapter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if adapter == nil {
		delete(s.adapters, kind)
		return
	}
	s.adapters[kind] = adapter
}

// Invoke 调用指定协议适配器，包含超时和重试逻辑。
func (s *AdapterService) Invoke(ctx context.Context, req *TransportRequest) (*TransportResponse, error) {
	adapter, err := s.getAdapter(req.Transport)
	if err != nil {
		return nil, err
	}

	attempt := 0
	maxAttempts := 1
	if req.RetryContext != nil && req.RetryContext.MaxAttempts > 1 {
		maxAttempts = req.RetryContext.MaxAttempts
	}

	var lastErr error
	var resp *TransportResponse

	for attempt < maxAttempts {
		attempt++

		callCtx, cancel := s.enforceDeadline(ctx, req)
		start := time.Now()
		spanName := fmt.Sprintf("transport.%s.%s", strings.ToLower(req.Transport.String()), req.CapabilityKey)
		if req.Observability != nil && req.Observability.SpanName != "" {
			spanName = req.Observability.SpanName
		}
		attributes := map[string]string{
			"capability.key":     req.CapabilityKey,
			"capability.version": req.Version,
			"transport":          strings.ToLower(req.Transport.String()),
		}
		for k, v := range req.Metadata {
			if strings.HasPrefix(k, "trace.") {
				attributes[k] = v
			}
		}
		callCtx, span := s.tracer.StartSpan(callCtx, spanName, attributes)

		resp, err = adapter.Invoke(callCtx, req)
		span.End(err)
		if cancel != nil {
			cancel()
		}

		if err == nil {
			if resp == nil {
				resp = &TransportResponse{}
			}
			resp.Attempt = attempt
			if resp.Error == nil {
				resp.Status = ResponseStatusSuccess
				s.metrics.ObserveInvocation(ctx, strings.ToLower(req.Transport.String()), req.CapabilityKey, time.Since(start), attempt, nil)
				return resp, nil
			}
			resp.Status = ResponseStatusError
			if !s.shouldRetryCapabilityError(resp.Error, req.RetryContext) || attempt >= maxAttempts {
				s.metrics.ObserveInvocation(ctx, strings.ToLower(req.Transport.String()), req.CapabilityKey, time.Since(start), attempt, resp.Error)
				return resp, resp.Error
			}
			lastErr = resp.Error
		} else {
			capErr := wrapError(err)
			lastErr = capErr
			if resp == nil {
				resp = &TransportResponse{}
			}
			resp.Attempt = attempt
			resp.Status = ResponseStatusError
			resp.Error = capErr
			s.metrics.ObserveInvocation(ctx, strings.ToLower(req.Transport.String()), req.CapabilityKey, time.Since(start), attempt, capErr)

			if !s.shouldRetry(err, req.RetryContext) || attempt >= maxAttempts {
				return resp, err
			}
		}

		delay := nextBackoff(attempt, req.RetryContext)
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return resp, ctx.Err()
			}
		}
	}

	if lastErr == nil {
		lastErr = ErrNoHealthyAdapter
	}
	return resp, lastErr
}

// InvokeWithFallback 按照传输偏好顺序进行调用，失败时自动降级。
func (s *AdapterService) InvokeWithFallback(ctx context.Context, req *TransportRequest, transports []capb.TransportKind) (*TransportResponse, error) {
	var lastErr error
	for _, transport := range transports {
		req.Transport = transport
		resp, err := s.Invoke(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = ErrNoHealthyAdapter
	}
	return nil, lastErr
}

// Stream 执行流式调用。
func (s *AdapterService) Stream(ctx context.Context, req *TransportRequest, sink chan<- *StreamChunk) error {
	adapter, err := s.getAdapter(req.Transport)
	if err != nil {
		return err
	}

	callCtx, cancel := s.enforceDeadline(ctx, req)
	defer func() {
		if cancel != nil {
			cancel()
		}
	}()

	spanName := fmt.Sprintf("transport.%s.stream.%s", strings.ToLower(req.Transport.String()), req.CapabilityKey)
	callCtx, span := s.tracer.StartSpan(callCtx, spanName, map[string]string{
		"capability.key": req.CapabilityKey,
		"transport":      strings.ToLower(req.Transport.String()),
	})
	defer span.End(nil)

	err = adapter.Stream(callCtx, req, sink)
	s.metrics.ObserveStream(ctx, strings.ToLower(req.Transport.String()), req.CapabilityKey, err)
	return err
}

// HealthCheck 调用适配器的健康检查并持久化结果。
func (s *AdapterService) HealthCheck(ctx context.Context, tenantID uint64, capabilityKey, version string, transport capb.TransportKind) (*TransportHealthReport, error) {
	contract, err := s.contractRepo.FindByKeyVersion(ctx, tenantID, capabilityKey, version, false)
	if err != nil {
		return nil, err
	}

	adapter, err := s.getAdapter(transport)
	if err != nil {
		return nil, err
	}

	report, err := adapter.HealthCheck(ctx, capabilityKey)
	s.metrics.RecordHealth(ctx, strings.ToLower(transport.String()), capabilityKey, healthStatus(report), err)
	if err != nil {
		return nil, err
	}
	if report == nil {
		report = &TransportHealthReport{Status: "unknown", CheckedAt: time.Now()}
	}

	payload, marshalErr := json.Marshal(report)
	if marshalErr == nil {
		_ = s.transportRepo.UpdateHealthStatus(ctx, contract.ID, transportToString(transport), datatypes.JSON(payload))
	}
	return report, nil
}

// Close 关闭所有注册的适配器。
func (s *AdapterService) Close(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var firstErr error
	for kind, adapter := range s.adapters {
		if adapter == nil {
			continue
		}
		if err := adapter.Close(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close adapter %s: %w", kind.String(), err)
		}
	}
	return firstErr
}

// ListProfiles 列出指定能力版本的传输配置。
func (s *AdapterService) ListProfiles(ctx context.Context, tenantID uint64, capabilityKey, version string) ([]TransportProfile, error) {
	contract, err := s.contractRepo.FindByKeyVersion(ctx, tenantID, capabilityKey, version, false)
	if err != nil {
		return nil, err
	}
	models, err := s.transportRepo.ListByContract(ctx, contract.ID)
	if err != nil {
		return nil, err
	}
	result := make([]TransportProfile, 0, len(models))
	for _, profile := range models {
		view := TransportProfile{
			Transport:        profile.Transport,
			Mode:             profile.Mode,
			TimeoutMillis:    profile.TimeoutMillis,
			Streaming:        profile.Streaming,
			Retry:            fromJSONMap(profile.RetryPolicy),
			QoS:              fromJSONMap(profile.QoS),
			EndpointSelector: fromJSONMap(profile.EndpointSelector),
			HealthReport:     ToHealthReport(profile.LastHealthStatus),
		}
		result = append(result, view)
	}
	return result, nil
}

// ReplaceProfiles 覆盖指定能力版本的传输配置。
func (s *AdapterService) ReplaceProfiles(ctx context.Context, tenantID uint64, capabilityKey, version string, profiles []capability.TransportProfile) error {
	contract, err := s.contractRepo.FindByKeyVersion(ctx, tenantID, capabilityKey, version, false)
	if err != nil {
		return err
	}
	models := toModelTransportProfiles(tenantID, contract.ID, capabilityKey, profiles)
	if err := s.transportRepo.DeleteByContract(ctx, contract.ID); err != nil {
		return err
	}
	if len(models) == 0 {
		return nil
	}
	return s.transportRepo.UpsertProfiles(ctx, models)
}

// UpdateHealthStatus 直接写入健康状态（用于外部上报）。
func (s *AdapterService) UpdateHealthStatus(ctx context.Context, tenantID uint64, capabilityKey, version string, transport capb.TransportKind, report *TransportHealthReport) error {
	contract, err := s.contractRepo.FindByKeyVersion(ctx, tenantID, capabilityKey, version, false)
	if err != nil {
		return err
	}
	if report == nil {
		report = &TransportHealthReport{Status: "unknown", CheckedAt: time.Now()}
	}
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}
	return s.transportRepo.UpdateHealthStatus(ctx, contract.ID, transportToString(transport), datatypes.JSON(data))
}

func (s *AdapterService) enforceDeadline(ctx context.Context, req *TransportRequest) (context.Context, context.CancelFunc) {
	if !req.Deadline.IsZero() {
		return context.WithDeadline(ctx, req.Deadline)
	}
	return ctx, nil
}

func (s *AdapterService) getAdapter(kind capb.TransportKind) (TransportAdapter, error) {
	s.mu.RLock()
	adapter, ok := s.adapters[kind]
	s.mu.RUnlock()
	if ok && adapter != nil {
		return adapter, nil
	}
	// 兼容大小写/字符串注册
	s.mu.RLock()
	defer s.mu.RUnlock()
	for k, candidate := range s.adapters {
		if candidate == nil {
			continue
		}
		if strings.EqualFold(k.String(), kind.String()) {
			return candidate, nil
		}
	}
	return nil, ErrAdapterNotFound
}

func (s *AdapterService) shouldRetry(err error, rc *RetryContext) bool {
	if rc == nil || !rc.Idempotent {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if rc.ShouldRetry != nil {
		return rc.ShouldRetry(err)
	}
	var r RetryableError
	if errors.As(err, &r) {
		return r.Retryable()
	}
	return false
}

func (s *AdapterService) shouldRetryCapabilityError(err *CapabilityError, rc *RetryContext) bool {
	if rc == nil || !rc.Idempotent || err == nil {
		return false
	}
	if rc.ShouldRetry != nil {
		return rc.ShouldRetry(err)
	}
	return err.Retryable
}

func nextBackoff(attempt int, rc *RetryContext) time.Duration {
	if rc == nil {
		return 0
	}
	initial := rc.InitialBackoff
	if initial <= 0 {
		initial = 100 * time.Millisecond
	}
	multiplier := rc.Multiplier
	if multiplier <= 1 {
		multiplier = 2
	}
	backoff := time.Duration(float64(initial) * mathPow(multiplier, float64(attempt-1)))
	if rc.MaxBackoff > 0 && backoff > rc.MaxBackoff {
		backoff = rc.MaxBackoff
	}
	return backoff
}

func mathPow(base, exp float64) float64 {
	if exp <= 0 {
		return 1
	}
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
}

func wrapError(err error) *CapabilityError {
	if err == nil {
		return nil
	}
	var capErr *CapabilityError
	if errors.As(err, &capErr) {
		return capErr
	}
	return &CapabilityError{
		Namespace: "transport",
		Category:  "runtime",
		Code:      "INTERNAL_ERROR",
		Severity:  "ERROR",
		Stage:     "invoke",
		Message:   err.Error(),
		Cause:     err,
		Retryable: false,
	}
}

func healthStatus(report *TransportHealthReport) string {
	if report == nil || report.Status == "" {
		return "unknown"
	}
	return report.Status
}

func transportToString(kind capb.TransportKind) string {
	switch kind {
	case capb.TransportKind_TRANSPORT_KIND_HTTP:
		return "http"
	case capb.TransportKind_TRANSPORT_KIND_GRPC:
		return "grpc"
	case capb.TransportKind_TRANSPORT_KIND_MCP:
		return "mcp"
	case capb.TransportKind_TRANSPORT_KIND_AGENT:
		return "agent"
	default:
		return "unspecified"
	}
}

type noopMetricsRecorder struct{}

func (n *noopMetricsRecorder) ObserveInvocation(context.Context, string, string, time.Duration, int, error) {
}
func (n *noopMetricsRecorder) ObserveStream(context.Context, string, string, error)        {}
func (n *noopMetricsRecorder) RecordHealth(context.Context, string, string, string, error) {}

type noopTracer struct{}

func (noopTracer) StartSpan(ctx context.Context, _ string, _ map[string]string) (context.Context, Span) {
	return ctx, noopSpan{}
}

type noopSpan struct{}

func (noopSpan) End(error) {}

// ToHealthReport 将模型中的 JSON 字段解析为健康报告。
func ToHealthReport(data datatypes.JSON) *TransportHealthReport {
	if len(data) == 0 {
		return nil
	}
	var report TransportHealthReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil
	}
	return &report
}
