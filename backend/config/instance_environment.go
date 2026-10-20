package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type environmentLookup func(string) (string, bool)

var instanceEnvironmentKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// configurationEnvironment 按显式配置目录读取 powerx.env，不修改进程环境。
// 已传入的进程变量优先；实例文件存在时，不搜索工作目录或其他实例的开发 .env。
func configurationEnvironment(configPath string) (environmentLookup, error) {
	values, found, err := readInstanceEnvironment(configPath)
	if err != nil {
		return nil, err
	}
	if !found {
		loadDotEnvCandidates(configPath)
	}
	return processEnvironmentOver(values), nil
}

func processEnvironmentOver(values map[string]string) environmentLookup {
	return func(name string) (string, bool) {
		if value, present := os.LookupEnv(name); present {
			return value, true
		}
		value, present := values[name]
		return value, present
	}
}

func readInstanceEnvironment(configPath string) (map[string]string, bool, error) {
	if strings.TrimSpace(configPath) == "" {
		return nil, false, nil
	}
	absolute, err := filepath.Abs(configPath)
	if err != nil {
		return nil, false, fmt.Errorf("实例配置路径无效")
	}
	path := filepath.Join(filepath.Dir(absolute), "powerx.env")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, fmt.Errorf("实例环境文件不可读: %s", path)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, true, fmt.Errorf("实例环境文件不是普通文件: %s", path)
	}
	values := map[string]string{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		entry := strings.TrimSpace(scanner.Text())
		if entry == "" || strings.HasPrefix(entry, "#") || strings.HasPrefix(entry, ";") {
			continue
		}
		entry = strings.TrimSpace(strings.TrimPrefix(entry, "export "))
		key, value, ok := strings.Cut(entry, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || !instanceEnvironmentKey.MatchString(key) {
			return nil, true, fmt.Errorf("实例环境文件格式错误: %s:%d", path, line)
		}
		if strings.HasPrefix(value, `"`) {
			value, err = strconv.Unquote(value)
			if err != nil {
				return nil, true, fmt.Errorf("实例环境文件引号无效: %s:%d", path, line)
			}
		} else if strings.HasPrefix(value, "'") {
			if len(value) < 2 || !strings.HasSuffix(value, "'") {
				return nil, true, fmt.Errorf("实例环境文件引号无效: %s:%d", path, line)
			}
			value = value[1 : len(value)-1]
		}
		// 不执行 shell、不展开 $VAR；值按字面保留，错误中不包含值。
		values[key] = value
	}
	if scanner.Err() != nil {
		return nil, true, fmt.Errorf("实例环境文件读取失败: %s", path)
	}
	return values, true, nil
}
