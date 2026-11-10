// build_fingerprint 为已完成编译的后端生成固定侧车清单。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/productinfo"
)

func main() {
	binary := flag.String("binary", "", "最终二进制路径")
	release := flag.String("release", "development", "发行标识")
	commit := flag.String("commit", "unknown", "源码提交")
	built := flag.String("built-at", time.Now().UTC().Format(time.RFC3339), "构建时间")
	targetOS := flag.String("os", runtime.GOOS, "目标系统")
	targetArch := flag.String("arch", runtime.GOARCH, "目标架构")
	flag.Parse()
	m, err := productinfo.GenerateBackend(*binary, *release, *commit, *built, *targetOS, *targetArch)
	if err == nil {
		var raw []byte
		raw, err = json.MarshalIndent(m, "", "  ")
		if err == nil {
			err = os.WriteFile(*binary+".build.json", append(raw, '\n'), 0644)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "生成构建清单失败:", err)
		os.Exit(1)
	}
}
