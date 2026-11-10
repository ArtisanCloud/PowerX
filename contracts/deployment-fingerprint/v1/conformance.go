// Package deploymentfingerprint 提供 M0 协议的一致性参考实现；不注册业务路由或修改数据库。
package deploymentfingerprint

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gowebpki/jcs"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// Bundle 是两端版本化的共同协议和样例，不包含运行实例资料。
//
//go:embed *.json
var Bundle embed.FS

const schemaID = "urn:powerx:deployment-fingerprint:v1"

var compilerOnce sync.Once
var compiler *jsonschema.Compiler
var compilerErr error
var compileMu sync.Mutex

// Validate 使用真实 JSON Schema 校验，并先拒绝重复 JSON 键和非法 Unicode。
func Validate(definition string, raw []byte) (map[string]any, error) {
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid strict JSON: %w", err)
	}
	var value map[string]any
	if err := json.Unmarshal(canonical, &value); err != nil {
		return nil, err
	}
	compilerOnce.Do(func() {
		compiler = jsonschema.NewCompiler()
		compiler.AssertFormat = true
		compiler.LoadURL = func(string) (io.ReadCloser, error) { return nil, fmt.Errorf("external schema loading disabled") }
		source, err := Bundle.ReadFile("protocol.schema.json")
		if err != nil {
			compilerErr = err
			return
		}
		compilerErr = compiler.AddResource(schemaID, bytes.NewReader(source))
	})
	if compilerErr != nil {
		return nil, compilerErr
	}
	compileMu.Lock()
	schema, err := compiler.Compile(schemaID + "#/$defs/" + definition)
	compileMu.Unlock()
	if err != nil {
		return nil, err
	}
	if err := schema.Validate(value); err != nil {
		return nil, err
	}
	return value, nil
}

func validArtifactPath(value string) bool {
	if value == "" || strings.ContainsAny(value, "\\%?#:\x00\r\n\t") || strings.HasPrefix(value, "/") || path.Clean(value) != value {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." || segment == "." || segment == "" {
			return false
		}
	}
	return true
}

// ManifestDigest 返回稳定载荷与域隔离 SHA256；不将 build_id 或 built_at 放入摘要。
func ManifestDigest(raw []byte) (string, string, error) {
	value, err := Validate("Manifest", raw)
	if err != nil {
		return "", "", err
	}
	artifacts := value["artifacts"].([]any)
	seen := map[string]bool{}
	for _, entry := range artifacts {
		a := entry.(map[string]any)
		p := a["path"].(string)
		if !validArtifactPath(p) || seen[p] {
			return "", "", fmt.Errorf("invalid or duplicate artifact path")
		}
		seen[p] = true
		if (value["component"] == "backend") != (a["kind"] == "binary") {
			return "", "", fmt.Errorf("component and artifact kind mismatch")
		}
	}
	sort.Slice(artifacts, func(i, j int) bool {
		a, b := artifacts[i].(map[string]any), artifacts[j].(map[string]any)
		if a["kind"] != b["kind"] {
			return a["kind"].(string) < b["kind"].(string)
		}
		// 数组的业务排序采用 UTF-8 字节顺序；对象属性的 JCS 排序仍由 RFC 8785 实现负责。
		return a["path"].(string) < b["path"].(string)
	})
	delete(value, "build_id")
	delete(value, "built_at")
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", "", err
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(append([]byte("powerx.build-fingerprint/v1\n"), canonical...))
	return string(canonical), hex.EncodeToString(sum[:]), nil
}

var asciiHost = regexp.MustCompile(`^[a-z0-9.-]+$`)
var utcMilliseconds = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{3})?Z$`)

// NormalizeSite 只规范化挑战使用的 HTTPS 单入口；DNS/出站限制仍由抓取器实现。
func NormalizeSite(raw string) (string, error) {
	return normalizeWebsite(raw, true)
}

// NormalizeWebsite 用于普通网站识别；HTTP 不满足实例关联的 HTTPS 门槛。
func NormalizeWebsite(raw string) (string, error) { return normalizeWebsite(raw, false) }

func normalizeWebsite(raw string, requireHTTPS bool) (string, error) {
	if strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\\%\x00\r\n\t?#") {
		return "", fmt.Errorf("ambiguous site URL")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && (requireHTTPS || u.Scheme != "http")) || u.Host == "" || u.User != nil || u.Opaque != "" {
		return "", fmt.Errorf("invalid HTTPS site")
	}
	host := strings.ToLower(u.Hostname())
	if ip := net.ParseIP(host); ip != nil {
		host = ip.String()
	} else {
		if !asciiHost.MatchString(host) || !strings.Contains(host, ".") || strings.Contains(host, "..") || strings.Trim(host, "0123456789.") == "" {
			return "", fmt.Errorf("ambiguous hostname")
		}
		for _, label := range strings.Split(host, ".") {
			if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") || strings.HasPrefix(label, "0x") {
				return "", fmt.Errorf("invalid hostname")
			}
		}
	}
	port := u.Port()
	defaultPort := "443"
	if u.Scheme == "http" {
		defaultPort = "80"
	}
	if port != "" && port != defaultPort {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("invalid port")
		}
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	u.Host = host
	if u.Path == "" {
		u.Path = "/"
	}
	if u.EscapedPath() != u.Path || path.Clean(u.Path) != strings.TrimSuffix(u.Path, "/") && u.Path != "/" {
		return "", fmt.Errorf("ambiguous site path")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/"
	return u.String(), nil
}

// RemainingMillis 仅处理服务端时间差和单调时钟经过量，不读取本机墙上时间。
func RemainingMillis(serverTime, expiresAt string, expiresIn, roundTrip int64, existing *int64) (int64, error) {
	if !utcMilliseconds.MatchString(serverTime) || !utcMilliseconds.MatchString(expiresAt) {
		return 0, fmt.Errorf("invalid UTC milliseconds")
	}
	server, err := time.Parse(time.RFC3339Nano, serverTime)
	if err != nil {
		return 0, err
	}
	expires, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return 0, err
	}
	if server.Nanosecond()%1000000 != 0 || expires.Nanosecond()%1000000 != 0 || expiresIn < 1 || expiresIn > 600000 || roundTrip < 0 || expires.Sub(server).Milliseconds() != expiresIn {
		return 0, fmt.Errorf("inconsistent challenge time")
	}
	remaining := expiresIn - roundTrip - 1000
	if existing != nil && *existing < remaining {
		remaining = *existing
	}
	if remaining < 0 {
		remaining = 0
	}
	return remaining, nil
}
