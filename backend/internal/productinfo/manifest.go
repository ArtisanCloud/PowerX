// Package productinfo 处理公开构建声明，不读取实例身份、租户或市场凭证。
package productinfo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gowebpki/jcs"
)

// Schema 是两端共同的构建清单版本。
const Schema = "powerx.build-fingerprint/v1"

// 由正式主程序的发布构建注入；开发模式不借用配置中的默认版本。
var ReleaseID = "development"

// SourceCommit 由发布构建注入，与实例身份无关。
var SourceCommit = "unknown"

// BuiltAt 是发布构建时间，不参与稳定摘要。
var BuiltAt = ""

// Artifact 声明一个最终制品的相对路径、大小及摘要。
type Artifact struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Target 声明制品目标平台。
type Target struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

// Manifest 是完整组件清单，不包含运行实例信息。
type Manifest struct {
	Schema       string     `json:"schema"`
	Product      string     `json:"product"`
	Component    string     `json:"component"`
	ReleaseID    string     `json:"release_id"`
	SourceCommit string     `json:"source_commit"`
	Target       Target     `json:"target"`
	Artifacts    []Artifact `json:"artifacts"`
	BuildID      string     `json:"build_id"`
	BuiltAt      string     `json:"built_at,omitempty"`
}

// BuildInfo 是登记与产品接口中的精简构建声明。
type BuildInfo struct {
	Schema       string `json:"schema"`
	Component    string `json:"component"`
	ReleaseID    string `json:"release_id"`
	SourceCommit string `json:"source_commit"`
	BuildID      string `json:"build_id"`
}

// Information 区分开发、缺失、无效和自检通过状态。
type Information struct {
	Product   string     `json:"product"`
	State     string     `json:"state"`
	BuildInfo *BuildInfo `json:"build_info,omitempty"`
}

func safePath(p string) bool {
	return p != "" && !strings.HasPrefix(p, "/") && path.Clean(p) == p && p != "." && p != ".." && !strings.HasPrefix(p, "../") && !strings.ContainsAny(p, "\\%?#:\x00\r\n\t")
}
func bounded(v string, n int) bool { return v != "" && len([]rune(v)) <= n }
func hash(v string) bool {
	b, e := hex.DecodeString(v)
	return e == nil && len(b) == 32 && v == strings.ToLower(v)
}

// CanonicalDigest 的数组采用 UTF-8 排序，对象采用 JCS，不包含 build_id/built_at。
func CanonicalDigest(m Manifest) (string, error) {
	if m.Schema != Schema || m.Product != "powerx" || (m.Component != "backend" && m.Component != "web-admin") || !bounded(m.ReleaseID, 128) || !bounded(m.SourceCommit, 128) || !bounded(m.Target.OS, 20) || !bounded(m.Target.Arch, 20) || len(m.Artifacts) < 1 || len(m.Artifacts) > 256 {
		return "", fmt.Errorf("invalid manifest")
	}
	if m.BuiltAt != "" {
		if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{3})?Z$`).MatchString(m.BuiltAt) {
			return "", fmt.Errorf("invalid build time")
		}
		if _, e := time.Parse(time.RFC3339Nano, m.BuiltAt); e != nil {
			return "", e
		}
	}
	seen := map[string]bool{}
	artifacts := append([]Artifact(nil), m.Artifacts...)
	for _, a := range artifacts {
		kind := "public_asset"
		if m.Component == "backend" {
			kind = "binary"
		}
		if a.Kind != kind || !safePath(a.Path) || seen[a.Path] || !hash(a.SHA256) || a.Size < 0 || a.Size > 9007199254740991 {
			return "", fmt.Errorf("invalid artifact")
		}
		seen[a.Path] = true
	}
	sort.Slice(artifacts, func(i, j int) bool {
		if artifacts[i].Kind != artifacts[j].Kind {
			return artifacts[i].Kind < artifacts[j].Kind
		}
		return artifacts[i].Path < artifacts[j].Path
	})
	payload := map[string]any{"schema": m.Schema, "product": m.Product, "component": m.Component, "release_id": m.ReleaseID, "source_commit": m.SourceCommit, "target": m.Target, "artifacts": artifacts}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte(Schema+"\n"), canonical...))
	return hex.EncodeToString(sum[:]), nil
}

// ParseManifest 拒绝重复键、未知字段及摘要不符的清单。
func ParseManifest(raw []byte) (Manifest, error) {
	var m Manifest
	if len(raw) > 256<<10 {
		return m, fmt.Errorf("manifest too large")
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return m, err
	}
	if err = validateSchema(canonical); err != nil {
		return m, err
	}
	d := json.NewDecoder(bytes.NewReader(canonical))
	d.DisallowUnknownFields()
	if err = d.Decode(&m); err != nil {
		return m, err
	}
	id, err := CanonicalDigest(m)
	if err != nil {
		return m, err
	}
	if !hash(m.BuildID) || id != m.BuildID {
		return m, fmt.Errorf("manifest digest mismatch")
	}
	return m, nil
}

// GenerateBackend 在二进制最终编译完成后生成侧车，不把自身摘要嵌回二进制。
func GenerateBackend(binary, release, commit, builtAt, targetOS, targetArch string) (Manifest, error) {
	m := Manifest{Schema: Schema, Product: "powerx", Component: "backend", ReleaseID: release, SourceCommit: commit, BuiltAt: builtAt, Target: Target{targetOS, targetArch}}
	f, err := os.Open(binary)
	if err != nil {
		return m, err
	}
	defer f.Close()
	sum := sha256.New()
	n, err := io.Copy(sum, f)
	if err != nil {
		return m, err
	}
	m.Artifacts = []Artifact{{Kind: "binary", Path: path.Base(binary), SHA256: hex.EncodeToString(sum.Sum(nil)), Size: n}}
	m.BuildID, err = CanonicalDigest(m)
	return m, err
}
