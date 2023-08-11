package zerox

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/zeromicro/go-zero/core/logx"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"
)

type LogConf struct {
	Path     string   `json:",default=logs"`
	Split    []string `json:",optional"`
	KeepDays int      `json:",default=30"`
	Level    string   `json:",default=info"`
	Console  bool     `json:",default=true"`
	Stat     bool     `json:",default=false"`
}

type Logger struct {
	serverLog io.WriteCloser
	errorLog  io.WriteCloser
	Level     LogLevel
}

type LogLevel int

const (
	timestampKey = "@timestamp"
	levelKey     = "level"
	contentKey   = "content"
)

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelError
)

var LogLevelMap = map[string]LogLevel{
	"debug": LevelDebug,
	"info":  LevelInfo,
	"error": LevelError,
}

func FromString(level string) LogLevel {
	if v, ok := LogLevelMap[level]; ok {
		return v
	}
	return LevelInfo
}

func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelError:
		return "error"
	}
	return "info"
}

func MustSetupLog(conf LogConf) {
	if !conf.Stat {
		logx.DisableStat()
	}
	if err := os.MkdirAll(conf.Path, os.ModePerm); err != nil {
		panic(err)
	}
	logger := NewLogger(conf)
	logx.SetWriter(logger)
}

type wrapConsoleWriter struct {
	console io.Writer
	file    io.WriteCloser
}

func (w wrapConsoleWriter) Write(p []byte) (n int, err error) {
	if _, err := w.console.Write(p); err != nil {
		return 0, err
	}
	if _, err := w.file.Write(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w wrapConsoleWriter) Close() error {
	return w.file.Close()
}

func NewLogger(conf LogConf) Logger {
	var logger Logger
	logger.serverLog = NewRotateLogger(filepath.Join(conf.Path, "server.log"), conf.KeepDays)
	if conf.Console {
		logger.serverLog = wrapConsoleWriter{
			console: os.Stdout,
			file:    logger.serverLog,
		}
	}
	if slices.Contains(conf.Split, "error") {
		logger.errorLog = NewRotateLogger(filepath.Join(conf.Path, "error.log"), conf.KeepDays)
		if conf.Console {
			logger.errorLog = wrapConsoleWriter{
				console: os.Stderr,
				file:    logger.errorLog,
			}
		}
	} else {
		logger.errorLog = logger.serverLog
	}
	logger.Level = FromString(conf.Level)
	return logger
}

func NewRotateLogger(filename string, days int) *logx.RotateLogger {
	rule := logx.DefaultRotateRule(filename, "-", days, false)
	rotate, err := logx.NewLogger(filename, rule, false)
	if err != nil {
		panic(err)
	}
	return rotate
}

func (l Logger) Alert(v interface{}) {
	if l.Level <= LevelError {
		output(l.errorLog, LevelInfo, v)
	}
}

func (l Logger) Close() error {
	l.errorLog.Close()
	l.serverLog.Close()
	return nil
}

func (l Logger) Debug(v interface{}, fields ...logx.LogField) {
	if l.Level <= LevelDebug {
		output(l.serverLog, LevelDebug, v, fields...)
	}
}

func (l Logger) Error(v interface{}, fields ...logx.LogField) {
	if l.Level <= LevelError {
		output(l.errorLog, LevelError, v, fields...)
	}
}

func (l Logger) Info(v interface{}, fields ...logx.LogField) {
	if l.Level <= LevelInfo {
		output(l.serverLog, LevelInfo, v, fields...)
	}
}

func (l Logger) Severe(v interface{}) {
	if l.Level <= LevelError {
		output(l.serverLog, LevelInfo, v)
	}
}

func (l Logger) Slow(v interface{}, fields ...logx.LogField) {
	if l.Level <= LevelError {
		output(l.serverLog, LevelInfo, v, fields...)
	}
}

func (l Logger) Stack(v interface{}) {
	if l.Level <= LevelError {
		output(l.errorLog, LevelError, v)
	}
}

func (l Logger) Stat(v interface{}, fields ...logx.LogField) {
	if l.Level <= LevelError {
		output(l.serverLog, LevelInfo, v, fields...)
	}
}

func output(writer io.Writer, level LogLevel, v interface{}, fields ...logx.LogField) {
	if writer == nil {
		return
	}
	levelStr := level.String()
	entry := make(map[string]interface{})
	for _, field := range fields {
		entry[field.Key] = field.Value
	}
	entry[levelKey] = levelStr
	entry[timestampKey] = time.Now().Format(time.RFC3339)
	entry[contentKey] = v
	if result, err := json.Marshal(entry); err != nil {
		fmt.Println(errors.New(fmt.Sprintf("marshal log entry failed: %v", err)))
	} else {
		_, _ = writer.Write(result)
	}
	writer.Write([]byte("\n"))
}
