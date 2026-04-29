package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger/config"
	lumberjack "github.com/ArtisanCloud/PowerX/pkg/utils/logger/lib"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger/runtimebuffer"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger/utils"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger/writer"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	driver *zap.Logger
	config config.LogConfig
}

// 全局变量用来存储 Logger 实例
var (
	once           sync.Once
	instance       *Logger
	globalInstance *Logger
	globalOnce     sync.Once
)

func getFileWriteSyncer(fileConfig *config.FileConfig, fileName string) zapcore.WriteSyncer {

	return zapcore.AddSync(&lumberjack.Logger{
		Filename:   fileName,
		MaxSize:    fileConfig.MaxSize, // megabytes
		MaxBackups: fileConfig.MaxBackups,
		MaxAge:     fileConfig.MaxAge,   // days
		Compress:   fileConfig.Compress, // disabled by default
	})
}

func NewLogger(config *config.LogConfig) *Logger {
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.LevelKey = config.Level // 保留 level
	if !config.ShowTimestamp {
		encoderConfig.TimeKey = "" // 不打印 timestamp
	} else {
		encoderConfig.TimeKey = "timestamp"
	}
	if !config.ShowCaller {
		encoderConfig.CallerKey = "" // 如果也不想要 caller，可以去掉
	}

	encoder := zapcore.NewConsoleEncoder(encoderConfig)
	if config.UseJsonFormat { // 如果需要 caller，可以去掉
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	var cores []zapcore.Core

	// Console 日志
	if config.Console {
		consoleCore := zapcore.NewCore(encoder, runtimebuffer.NewTeeWriteSyncer(zapcore.AddSync(os.Stdout)), config.ParseLogLevel())
		cores = append(cores, consoleCore)
	}

	// 文件日志
	if config.File.Enable {
		if config.File.InfoFilePath == "" {
			config.File.InfoFilePath = "logs/info.log"
		}
		utils.EnsureFileExists(config.File.InfoFilePath)
		if config.File.ErrorFilePath == "" {
			config.File.ErrorFilePath = "logs/error.log"
		}
		utils.EnsureFileExists(config.File.ErrorFilePath)

		// 只记录 Info 及以上（但不包括 Error）级别的日志
		infoLevelEnabler := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl >= zapcore.InfoLevel && lvl < zapcore.ErrorLevel
		})
		infoFileCore := zapcore.NewCore(encoder, getFileWriteSyncer(&config.File, config.File.InfoFilePath), infoLevelEnabler)
		cores = append(cores, infoFileCore)

		// 只记录 Error 及以上级别的日志
		errorLevelEnabler := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl >= zapcore.ErrorLevel
		})
		errorFileCore := zapcore.NewCore(encoder, getFileWriteSyncer(&config.File, config.File.ErrorFilePath), errorLevelEnabler)
		cores = append(cores, errorFileCore)
	}

	// Loki 日志
	if config.Loki.Enable {
		lokiCore := zapcore.NewCore(encoder, writer.NewLokiWriter(&config.Loki), config.ParseLogLevel())
		cores = append(cores, lokiCore)
	}

	// 组合多个日志 Core
	core := zapcore.NewTee(cores...)

	// 创建 Logger
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	return &Logger{
		driver: logger,
		config: *config,
	}
}

func GetLogger(c *config.LogConfig) *Logger {
	if c == nil {
		c = &config.LogConfig{
			Level:         "debug",
			Console:       true,
			UseJsonFormat: false,
			File: config.FileConfig{
				Enable: false,
			},
			Loki: config.LokiConfig{
				Enable: false,
			},
			HttpDebug: true,
			Debug:     true,
		}
	}

	once.Do(func() {
		// 仅在第一次调用时初始化
		instance = NewLogger(c)
	})
	return instance
}

func (l *Logger) WithContext(ctx context.Context) *Logger {
	// 从 context 中提取常用的字段
	fields := l.extractFieldsFromContext(ctx)
	if len(fields) > 0 {
		// 创建一个带有 context 字段的新 logger
		return &Logger{
			driver: l.driver.With(fields...),
			config: l.config,
		}
	}
	return l
}

// extractFieldsFromContext 从 context 中提取日志字段
func (l *Logger) extractFieldsFromContext(ctx context.Context) []zap.Field {
	var fields []zap.Field

	if ctx == nil {
		return fields
	}

	// 提取请求ID
	if requestID := ctx.Value("request_id"); requestID != nil {
		if id, ok := requestID.(string); ok && id != "" {
			fields = append(fields, zap.String("request_id", id))
		}
	}

	// 提取用户ID
	if userID := ctx.Value("user_id"); userID != nil {
		if id, ok := userID.(string); ok && id != "" {
			fields = append(fields, zap.String("user_id", id))
		}
	}

	// 提取会话ID
	if sessionID := ctx.Value("session_id"); sessionID != nil {
		if id, ok := sessionID.(string); ok && id != "" {
			fields = append(fields, zap.String("session_id", id))
		}
	}

	// 提取跟踪ID
	if traceID := ctx.Value("trace_id"); traceID != nil {
		if id, ok := traceID.(string); ok && id != "" {
			fields = append(fields, zap.String("trace_id", id))
		}
	}

	// 提取操作名称
	if operation := ctx.Value("operation"); operation != nil {
		if op, ok := operation.(string); ok && op != "" {
			fields = append(fields, zap.String("operation", op))
		}
	}

	// 提取租户与插件维度
	if tenantUUID := ctx.Value("tenant_uuid"); tenantUUID != nil {
		if v, ok := tenantUUID.(string); ok && v != "" {
			fields = append(fields, zap.String("tenant_uuid", v))
		}
	}
	if pluginID := ctx.Value("plugin_id"); pluginID != nil {
		if v, ok := pluginID.(string); ok && v != "" {
			fields = append(fields, zap.String("plugin_id", v))
		}
	}
	if messageID := ctx.Value("message_id"); messageID != nil {
		if v, ok := messageID.(string); ok && v != "" {
			fields = append(fields, zap.String("message_id", v))
		}
	}

	// 提取附加业务字段（高基数字段请放这里，不要进 Loki labels）
	if raw := ctx.Value(contextLogFieldsKey); raw != nil {
		if extra, ok := raw.(map[string]interface{}); ok {
			fields = append(fields, toZapFields(extra)...)
		}
	}

	return fields
}

func toZapFields(in map[string]interface{}) []zap.Field {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		if strings.TrimSpace(k) == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]zap.Field, 0, len(keys))
	for _, k := range keys {
		v := in[k]
		switch vv := v.(type) {
		case string:
			out = append(out, zap.String(k, vv))
		case bool:
			out = append(out, zap.Bool(k, vv))
		case int:
			out = append(out, zap.Int(k, vv))
		case int32:
			out = append(out, zap.Int32(k, vv))
		case int64:
			out = append(out, zap.Int64(k, vv))
		case uint:
			out = append(out, zap.Uint(k, vv))
		case uint32:
			out = append(out, zap.Uint32(k, vv))
		case uint64:
			out = append(out, zap.Uint64(k, vv))
		case float32:
			out = append(out, zap.Float32(k, vv))
		case float64:
			out = append(out, zap.Float64(k, vv))
		default:
			out = append(out, zap.Any(k, vv))
		}
	}
	return out
}

// PrettyJson 美化 JSON 输出
func (l *Logger) PrettyJson(data interface{}) (string, error) {
	// 使用 json.MarshalIndent 替代 bytes.Buffer 方式
	jsonBytes, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// formatMessage 格式化消息，支持结构化数据的美化输出
func (l *Logger) formatMessage(format string, args ...interface{}) string {
	// 如果只有一个参数且是结构体或复杂对象，尝试美化 JSON 输出
	if len(args) == 1 && l.shouldPrettyPrint(format, args[0]) {
		if prettyJson, err := l.PrettyJson(args[0]); err == nil {
			// 移除末尾的换行符
			prettyJson = strings.TrimSuffix(prettyJson, "\n")

			// 直接替换格式字符串，统一由 logger sink 输出（console/file/loki）
			if strings.Contains(format, "%+v") {
				return strings.ReplaceAll(format, "%+v", "\n"+prettyJson)
			}
			if strings.Contains(format, "%v") {
				return strings.ReplaceAll(format, "%v", "\n"+prettyJson)
			}
		}
	}

	// 默认使用标准格式化
	return fmt.Sprintf(format, args...)
}

// shouldPrettyPrint 判断是否应该进行美化输出
func (l *Logger) shouldPrettyPrint(format string, arg interface{}) bool {
	// 检查格式字符串是否包含 %+v 或 %v
	if !strings.Contains(format, "%+v") && !strings.Contains(format, "%v") {
		return false
	}

	// 检查参数类型
	if arg == nil {
		return false
	}

	v := reflect.ValueOf(arg)
	switch v.Kind() {
	case reflect.Struct, reflect.Map, reflect.Slice, reflect.Array:
		return true
	case reflect.Ptr:
		if v.IsNil() {
			return false
		}
		elem := v.Elem()
		return elem.Kind() == reflect.Struct || elem.Kind() == reflect.Map ||
			elem.Kind() == reflect.Slice || elem.Kind() == reflect.Array
	default:
		return false
	}
}

// Info 输出 Info 日志 - 支持 context 作为第一个参数
func (l *Logger) Info(ctx context.Context, msg string, fields ...zap.Field) {
	l.WithContext(ctx).driver.Info(msg, fields...)
}

// Error 输出 Error 日志 - 支持 context 作为第一个参数
func (l *Logger) Error(ctx context.Context, msg string, fields ...zap.Field) {
	l.WithContext(ctx).driver.Error(msg, fields...)
}

// Debug 输出 Debug 日志 - 支持 context 作为第一个参数
func (l *Logger) Debug(ctx context.Context, msg string, fields ...zap.Field) {
	l.WithContext(ctx).driver.Debug(msg, fields...)
}

// Warn 输出 Warn 日志 - 支持 context 作为第一个参数
func (l *Logger) Warn(ctx context.Context, msg string, fields ...zap.Field) {
	l.WithContext(ctx).driver.Warn(msg, fields...)
}

// InfoF 格式化后输出 Info 日志 - 支持 context 作为第一个参数
func (l *Logger) InfoF(ctx context.Context, format string, args ...interface{}) {
	message := l.formatMessage(format, args...)
	if message != "" {
		l.WithContext(ctx).driver.Info(message)
	}
}

// ErrorF 格式化后输出 Error 日志 - 支持 context 作为第一个参数
func (l *Logger) ErrorF(ctx context.Context, format string, args ...interface{}) {
	message := l.formatMessage(format, args...)
	if message != "" {
		l.WithContext(ctx).driver.Error(message)
	}
}

// DebugF 格式化后输出 Debug 日志 - 支持 context 作为第一个参数
func (l *Logger) DebugF(ctx context.Context, format string, args ...interface{}) {
	message := l.formatMessage(format, args...)
	if message != "" {
		l.WithContext(ctx).driver.Debug(message)
	}
}

// WarnF 格式化后输出 Warn 日志 - 支持 context 作为第一个参数
func (l *Logger) WarnF(ctx context.Context, format string, args ...interface{}) {
	message := l.formatMessage(format, args...)
	if message != "" {
		l.WithContext(ctx).driver.Warn(message)
	}
}

// InitGlobalLogger 初始化全局Logger实例
func InitGlobalLogger(config *config.LogConfig) {
	globalOnce.Do(func() {
		globalInstance = NewLogger(config)
	})
}

// GetGlobalLogger 获取全局Logger实例
func GetGlobalLogger() *Logger {
	if globalInstance == nil {
		// 如果没有初始化，使用默认配置
		InitGlobalLogger(&config.LogConfig{
			Level:         "debug",
			Console:       true,
			UseJsonFormat: false,
			File: config.FileConfig{
				Enable: false,
			},
			Loki: config.LokiConfig{
				Enable: false,
			},
			HttpDebug: true,
			Debug:     true,
		})
	}
	return globalInstance
}

// 全局便捷函数 - 支持 context 作为第一个参数

// Info 全局Info日志
func Info(ctx context.Context, msg string, fields ...zap.Field) {
	GetGlobalLogger().Info(ctx, msg, fields...)
}

// Error 全局Error日志
func Error(ctx context.Context, msg string, fields ...zap.Field) {
	GetGlobalLogger().Error(ctx, msg, fields...)
}

// Debug 全局Debug日志
func Debug(ctx context.Context, msg string, fields ...zap.Field) {
	GetGlobalLogger().Debug(ctx, msg, fields...)
}

// Warn 全局Warn日志
func Warn(ctx context.Context, msg string, fields ...zap.Field) {
	GetGlobalLogger().Warn(ctx, msg, fields...)
}

// InfoF 全局格式化Info日志
func InfoF(ctx context.Context, format string, args ...interface{}) {
	GetGlobalLogger().InfoF(ctx, format, args...)
}

// ErrorF 全局格式化Error日志
func ErrorF(ctx context.Context, format string, args ...interface{}) {
	GetGlobalLogger().ErrorF(ctx, format, args...)
}

// DebugF 全局格式化Debug日志
func DebugF(ctx context.Context, format string, args ...interface{}) {
	GetGlobalLogger().DebugF(ctx, format, args...)
}

// WarnF 全局格式化Warn日志
func WarnF(ctx context.Context, format string, args ...interface{}) {
	GetGlobalLogger().WarnF(ctx, format, args...)
}
