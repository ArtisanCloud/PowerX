package utils

import (
	"os"
	"path/filepath"
)

// EnsureDir 确保目录存在，如果不存在则创建
func EnsureDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}

// FileExists 检查文件是否存在
func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

// GetFileSize 获取文件大小
func GetFileSize(filename string) (int64, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// GetLogDir 获取日志目录
func GetLogDir(filename string) string {
	if filename == "" {
		return "./logs"
	}
	return filepath.Dir(filename)
}

// GetLogFileName 获取日志文件名
func GetLogFileName(filename string) string {
	if filename == "" {
		return "app.log"
	}
	return filepath.Base(filename)
}

// EnsureFileExists 确保文件存在，如果不存在则创建
func EnsureFileExists(filename string) error {
	// 确保目录存在
	dir := filepath.Dir(filename)
	if err := EnsureDir(dir); err != nil {
		return err
	}

	// 如果文件不存在，创建空文件
	if !FileExists(filename) {
		file, err := os.Create(filename)
		if err != nil {
			return err
		}
		file.Close()
	}

	return nil
}
