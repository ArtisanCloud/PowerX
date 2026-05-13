package config

import (
	"go.uber.org/zap/zapcore"
)

// LogConfig 日志配置
type LogConfig struct {
	Level         string           `yaml:"level"`           // 日志级别: debug, info, warn, error
	Console       bool             `yaml:"console"`         // 是否输出到控制台
	UseJsonFormat bool             `yaml:"use_json_format"` // 是否使用JSON格式
	ShowTimestamp bool             `yaml:"show_timestamp"`  // 是否显示时间戳
	ShowCaller    bool             `yaml:"show_caller"`     // 是否显示调用者信息
	File          FileConfig       `yaml:"file"`            // 文件输出配置
	Loki          LokiConfig       `yaml:"loki"`            // Loki输出配置
	Retention     RetentionConfig  `yaml:"retention"`       // 统一日志保留配置
	AgentDebug    AgentDebugConfig `yaml:"agent_debug"`     // Agent 运行时调试落盘
	HttpDebug     bool             `yaml:"http_debug"`      // HTTP调试日志
	Debug         bool             `yaml:"debug"`           // 调试模式
}

// FileConfig 文件日志配置
type FileConfig struct {
	Enable        bool   `yaml:"enable"`          // 是否启用文件日志
	InfoFilePath  string `yaml:"info_file_path"`  // 信息日志文件路径
	ErrorFilePath string `yaml:"error_file_path"` // 错误日志文件路径
	MaxSize       int    `yaml:"max_size"`        // 单个文件最大大小(MB)
	MaxBackups    int    `yaml:"max_backups"`     // 保留的备份文件数量
	MaxAge        int    `yaml:"max_age"`         // 保留的天数
	Compress      bool   `yaml:"compress"`        // 是否压缩备份文件
	RotateDaily   bool   `yaml:"rotate_daily"`    // 是否按天切分（跨天首次写入触发）
}

// LokiConfig Loki日志配置
type LokiConfig struct {
	Enable    bool              `yaml:"enable"`     // 是否启用Loki
	URL       string            `yaml:"url"`        // Loki服务器地址
	Labels    map[string]string `yaml:"labels"`     // 低基数标签（system/service/env/instance/module）
	BatchWait int               `yaml:"batch_wait"` // 批量等待时间(秒)
	BatchSize int               `yaml:"batch_size"` // 批量大小
}

// AgentDebugConfig 控制 Agent 单请求调试追踪文件。
type AgentDebugConfig struct {
	Enable       bool   `yaml:"enable"`
	Dir          string `yaml:"dir"`
	MaxBodyBytes int    `yaml:"max_body_bytes"`
}

// RetentionConfig 统一日志保留与清理配置。
type RetentionConfig struct {
	Enabled              bool               `yaml:"enabled"`
	Cron                 string             `yaml:"cron"`
	Timezone             string             `yaml:"timezone"`
	DefaultRetentionDays int                `yaml:"default_retention_days"`
	FilePaths            []string           `yaml:"file_paths"`
	DBTables             []RetentionDBTable `yaml:"db_tables"`
	BatchSize            int                `yaml:"batch_size"`
	MaxDeleteRowsPerRun  int                `yaml:"max_delete_rows_per_run"`
}

// RetentionDBTable 指定数据库日志表的清理规则。
type RetentionDBTable struct {
	Name          string `yaml:"name"`
	TimeColumn    string `yaml:"time_column"`
	RetentionDays int    `yaml:"retention_days"`
}

// ParseLogLevel 解析日志级别
func (c *LogConfig) ParseLogLevel() zapcore.Level {
	switch c.Level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}
