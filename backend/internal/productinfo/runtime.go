package productinfo

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// LoadExecutable 只读取当前可执行文件旁的固定侧车，任何失败都不阻塞启动。
func LoadExecutable() Information {
	executable, err := os.Executable()
	if err != nil {
		return Information{Product: "powerx", State: "missing"}
	}
	if resolved, e := filepath.EvalSymlinks(executable); e == nil {
		executable = resolved
	}
	return LoadAt(executable, ReleaseID == "development")
}

// LoadAt 检查当前实际二进制，不把声明摘要当作远程验真。
func LoadAt(executable string, development bool) Information {
	out := Information{Product: "powerx", State: "missing"}
	f, err := os.Open(executable + ".build.json")
	if os.IsNotExist(err) && development {
		// go run 没有发行侧车，也应自动报告当前真实程序的构建摘要。
		// 保留 development 状态，不将本地构建提升为已验证的官方发行包。
		m, err := GenerateBackend(executable, "development", SourceCommit, "", runtime.GOOS, runtime.GOARCH)
		if err != nil {
			out.State = "invalid"
			return out
		}
		out.State = "development"
		out.BuildInfo = &BuildInfo{m.Schema, m.Component, m.ReleaseID, m.SourceCommit, m.BuildID}
		return out
	}
	if err != nil {
		return out
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (256<<10)+1))
	if err != nil {
		out.State = "invalid"
		return out
	}
	m, err := ParseManifest(raw)
	if err != nil || m.Component != "backend" || m.Target.OS != runtime.GOOS || m.Target.Arch != runtime.GOARCH || len(m.Artifacts) != 1 || m.Artifacts[0].Path != filepath.Base(executable) {
		out.State = "invalid"
		return out
	}
	if !development && (m.ReleaseID != ReleaseID || m.SourceCommit != SourceCommit) {
		out.State = "invalid"
		return out
	}
	binary, err := os.Open(executable)
	if err != nil {
		out.State = "invalid"
		return out
	}
	defer binary.Close()
	sum := sha256.New()
	size, err := io.Copy(sum, binary)
	if err != nil || size != m.Artifacts[0].Size || hex.EncodeToString(sum.Sum(nil)) != m.Artifacts[0].SHA256 {
		out.State = "digest_mismatch"
		return out
	}
	out.State = "available"
	out.BuildInfo = &BuildInfo{m.Schema, m.Component, m.ReleaseID, m.SourceCommit, m.BuildID}
	return out
}
