package writer

import (
	"fmt"
	lumberjack "github.com/ArtisanCloud/PowerX/pkg/utils/logger/lib"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger/utils"

	"go.uber.org/zap/zapcore"
)

// FileWriter 文件写入器配置
type FileWriter struct {
	Filename   string `json:"filename" yaml:"filename"`     // 日志文件路径
	MaxSize    int    `json:"maxsize" yaml:"maxsize"`       // 单个文件最大大小(MB)
	MaxAge     int    `json:"maxage" yaml:"maxage"`         // 文件最大保存天数
	MaxBackups int    `json:"maxbackups" yaml:"maxbackups"` // 最大备份文件数量
	LocalTime  bool   `json:"localtime" yaml:"localtime"`   // 是否使用本地时间
	Compress   bool   `json:"compress" yaml:"compress"`     // 是否压缩备份文件
}

// NewFileWriter 创建新的文件写入器
func NewFileWriter(config FileWriter) (zapcore.WriteSyncer, error) {
	// 确保日志目录存在
	logDir := utils.GetLogDir(config.Filename)
	if err := utils.EnsureDir(logDir); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %v", err)
	}

	// 设置默认值
	if config.MaxSize == 0 {
		config.MaxSize = 100 // 默认100MB
	}
	if config.MaxAge == 0 {
		config.MaxAge = 30 // 默认保存30天
	}
	if config.MaxBackups == 0 {
		config.MaxBackups = 10 // 默认保留10个备份文件
	}

	// 创建lumberjack logger
	lumberjackLogger := &lumberjack.Logger{
		Filename:   config.Filename,
		MaxSize:    config.MaxSize,
		MaxAge:     config.MaxAge,
		MaxBackups: config.MaxBackups,
		LocalTime:  config.LocalTime,
		Compress:   config.Compress,
	}

	return zapcore.AddSync(lumberjackLogger), nil
}

// GetDefaultFileWriter 获取默认的文件写入器配置
func GetDefaultFileWriter() FileWriter {
	return FileWriter{
		Filename:   "./logs/app.log",
		MaxSize:    100,
		MaxAge:     30,
		MaxBackups: 10,
		LocalTime:  true,
		Compress:   true,
	}
}
