package manager

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
)

var (
	// ErrDriverNotFound 表示指定的驱动不存在。
	ErrDriverNotFound = errors.New("media manager: driver not found")
	// ErrNoDefaultDriver 表示尚未配置默认驱动。
	ErrNoDefaultDriver = errors.New("media manager: default driver not configured")
)

type operationType string

const (
	operationPut     operationType = "put"
	operationGet     operationType = "get"
	operationDelete  operationType = "delete"
	operationPresign operationType = "presign"
)

type operationMetrics struct {
	calls        int64
	failures     int64
	latencyNanos int64
}

type metricsCollector struct {
	mu      sync.RWMutex
	metrics map[string]map[operationType]*operationMetrics
}

func newMetricsCollector() *metricsCollector {
	return &metricsCollector{
		metrics: make(map[string]map[operationType]*operationMetrics),
	}
}

func (c *metricsCollector) record(driverName string, op operationType, duration time.Duration, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ops, ok := c.metrics[driverName]
	if !ok {
		ops = make(map[operationType]*operationMetrics)
		c.metrics[driverName] = ops
	}
	metric, ok := ops[op]
	if !ok {
		metric = &operationMetrics{}
		ops[op] = metric
	}
	metric.calls++
	metric.latencyNanos += duration.Nanoseconds()
	if err != nil {
		metric.failures++
	}
}

// OperationSnapshot 为指标快照，方便外部采集。
type OperationSnapshot struct {
	Calls        int64
	Failures     int64
	AvgLatency   time.Duration
	TotalLatency time.Duration
}

// DriverSnapshot 描述单个驱动的指标集合。
type DriverSnapshot struct {
	Driver    string
	Metrics   map[string]OperationSnapshot
	LastError error
}

func (c *metricsCollector) snapshot(lastErrors map[string]error) []DriverSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	drivers := make([]DriverSnapshot, 0, len(c.metrics))
	for driverName, ops := range c.metrics {
		snapshot := DriverSnapshot{
			Driver:  driverName,
			Metrics: make(map[string]OperationSnapshot, len(ops)),
		}
		for op, metric := range ops {
			avg := time.Duration(0)
			if metric.calls > 0 && metric.latencyNanos > 0 {
				avg = time.Duration(metric.latencyNanos / metric.calls)
			}
			snapshot.Metrics[string(op)] = OperationSnapshot{
				Calls:        metric.calls,
				Failures:     metric.failures,
				AvgLatency:   avg,
				TotalLatency: time.Duration(metric.latencyNanos),
			}
		}
		if lastErr, ok := lastErrors[driverName]; ok {
			snapshot.LastError = lastErr
		}
		drivers = append(drivers, snapshot)
	}

	sort.Slice(drivers, func(i, j int) bool { return drivers[i].Driver < drivers[j].Driver })
	return drivers
}

// MediaManager 负责驱动注册、默认驱动管理、指标收集与健康检查。
type MediaManager struct {
	mu             sync.RWMutex
	drivers        map[string]driver.StorageDriver
	defaultDriver  string
	metrics        *metricsCollector
	lastErrorByDrv map[string]error
}

// New 创建媒体驱动管理器。
func New(defaultDriver string) *MediaManager {
	return &MediaManager{
		drivers:        make(map[string]driver.StorageDriver),
		defaultDriver:  strings.TrimSpace(defaultDriver),
		metrics:        newMetricsCollector(),
		lastErrorByDrv: make(map[string]error),
	}
}

// RegisterDriver 注册驱动，若同名驱动已存在则覆盖。
func (m *MediaManager) RegisterDriver(drv driver.StorageDriver) {
	if drv == nil {
		return
	}
	name := strings.TrimSpace(drv.Name())
	if name == "" {
		name = "default"
	}

	m.mu.Lock()
	m.drivers[name] = drv
	m.mu.Unlock()
}

// SetDefaultDriver 设置默认驱动，必须是已注册的驱动名称。
func (m *MediaManager) SetDefaultDriver(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ErrDriverNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.drivers[trimmed]; !ok {
		return ErrDriverNotFound
	}
	m.defaultDriver = trimmed
	return nil
}

// DefaultDriver 返回当前默认驱动名称。
func (m *MediaManager) DefaultDriver() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.defaultDriver == "" {
		return "", ErrNoDefaultDriver
	}
	if _, ok := m.drivers[m.defaultDriver]; !ok {
		return "", ErrNoDefaultDriver
	}
	return m.defaultDriver, nil
}

func (m *MediaManager) resolveDriver(name string) (string, driver.StorageDriver, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if name != "" {
		drv, ok := m.drivers[name]
		if !ok {
			return "", nil, ErrDriverNotFound
		}
		return name, drv, nil
	}
	if m.defaultDriver == "" {
		return "", nil, ErrNoDefaultDriver
	}
	drv, ok := m.drivers[m.defaultDriver]
	if !ok {
		return "", nil, ErrNoDefaultDriver
	}
	return m.defaultDriver, drv, nil
}

// Drivers 返回已注册驱动的名称列表。
func (m *MediaManager) Drivers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.drivers))
	for name := range m.drivers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Put 将对象写入指定驱动。
func (m *MediaManager) Put(ctx context.Context, driverName string, in driver.PutObjectInput) (*driver.PutObjectResult, error) {
	resolved, drv, err := m.resolveDriver(driverName)
	if err != nil {
		m.recordError(resolved, err)
		return nil, err
	}
	start := time.Now()
	result, opErr := drv.Put(ctx, in)
	m.metrics.record(resolved, operationPut, time.Since(start), opErr)
	m.recordError(resolved, opErr)
	return result, opErr
}

// Get 读取对象内容。
func (m *MediaManager) Get(ctx context.Context, driverName string, in driver.GetObjectInput) (*driver.GetObjectResult, error) {
	resolved, drv, err := m.resolveDriver(driverName)
	if err != nil {
		m.recordError(resolved, err)
		return nil, err
	}
	start := time.Now()
	result, opErr := drv.Get(ctx, in)
	m.metrics.record(resolved, operationGet, time.Since(start), opErr)
	m.recordError(resolved, opErr)
	return result, opErr
}

// Delete 删除指定对象。
func (m *MediaManager) Delete(ctx context.Context, driverName string, in driver.DeleteObjectInput) error {
	resolved, drv, err := m.resolveDriver(driverName)
	if err != nil {
		m.recordError(resolved, err)
		return err
	}
	start := time.Now()
	opErr := drv.Delete(ctx, in)
	m.metrics.record(resolved, operationDelete, time.Since(start), opErr)
	m.recordError(resolved, opErr)
	return opErr
}

// GenerateURL 生成访问或上传的临时地址。
func (m *MediaManager) GenerateURL(ctx context.Context, driverName string, in driver.GenerateURLInput) (*driver.GenerateURLOutput, error) {
	resolved, drv, err := m.resolveDriver(driverName)
	if err != nil {
		m.recordError(resolved, err)
		return nil, err
	}
	start := time.Now()
	result, opErr := drv.GenerateURL(ctx, in)
	m.metrics.record(resolved, operationPresign, time.Since(start), opErr)
	m.recordError(resolved, opErr)
	return result, opErr
}

// HealthCheck 依次检查所有驱动健康状态，返回失败列表（成功返回空 map）。
func (m *MediaManager) HealthCheck(ctx context.Context) map[string]error {
	m.mu.RLock()
	drivers := make(map[string]driver.StorageDriver, len(m.drivers))
	for name, drv := range m.drivers {
		drivers[name] = drv
	}
	m.mu.RUnlock()

	failed := make(map[string]error)
	for name, drv := range drivers {
		if err := drv.HealthCheck(ctx); err != nil {
			failed[name] = fmt.Errorf("driver %s unhealthy: %w", name, err)
			m.recordError(name, err)
		} else {
			m.recordError(name, nil)
		}
	}
	return failed
}

// MetricsSnapshot 返回当前的指标快照，方便 Prometheus/日志导出。
func (m *MediaManager) MetricsSnapshot() []DriverSnapshot {
	m.mu.RLock()
	lastErr := make(map[string]error, len(m.lastErrorByDrv))
	for k, v := range m.lastErrorByDrv {
		lastErr[k] = v
	}
	m.mu.RUnlock()
	return m.metrics.snapshot(lastErr)
}

func (m *MediaManager) recordError(driverName string, err error) {
	if driverName == "" {
		return
	}
	m.mu.Lock()
	if err == nil {
		delete(m.lastErrorByDrv, driverName)
	} else {
		m.lastErrorByDrv[driverName] = err
	}
	m.mu.Unlock()
}

// EnsureDriver 确认驱动是否存在，若不存在返回 ErrDriverNotFound。
func (m *MediaManager) EnsureDriver(name string) error {
	_, _, err := m.resolveDriver(name)
	return err
}
