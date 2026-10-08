package main

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
)

// 发布构建通过 -ldflags 注入；普通 go build 仍可读取 Go 自带的 VCS 信息。
var version = "unversioned"
var commit string
var buildTime string

var databaseCommands = []string{
	"migrate", "seed", "seed-native-marketing-skills", "refresh", "status",
	"iam-report", "iam-fix-owner", "iam-fix-role-binding-duplicates", "repair-agent-run-state",
	"repair-plugin-runtime-credentials", "prepare-plugin-runtime-credentials",
}

type databaseVersion struct {
	Program        string   `json:"program"`
	Version        string   `json:"version"`
	GitCommit      string   `json:"git_commit"`
	BuildTime      string   `json:"build_time"`
	SourceModified *bool    `json:"source_modified"`
	GoVersion      string   `json:"go_version"`
	Commands       []string `json:"commands"`
}

func databaseVersionInformation(info *debug.BuildInfo) databaseVersion {
	out := databaseVersion{
		Program: "database", Version: version, GitCommit: commit, BuildTime: buildTime,
		GoVersion: runtime.Version(), Commands: append([]string(nil), databaseCommands...),
	}
	if out.GitCommit == "" {
		out.GitCommit = "unknown"
	}
	if out.BuildTime == "" {
		out.BuildTime = "unknown"
	}
	if info != nil {
		out.GoVersion = info.GoVersion
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				out.GitCommit = setting.Value
			case "vcs.modified":
				if setting.Value == "true" || setting.Value == "false" {
					modified := setting.Value == "true"
					out.SourceModified = &modified
				}
			}
		}
	}
	return out
}

// handleDatabaseInformation 在加载配置和连接数据库之前处理只读 CLI 信息查询。
func handleDatabaseInformation(args []string, output io.Writer) (bool, error) {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(output, "Usage: database <command> [flags]\nCommands:")
		if err != nil {
			return true, err
		}
		for _, cmd := range databaseCommands {
			if _, err := fmt.Fprintln(output, "  "+cmd); err != nil {
				return true, err
			}
		}
		_, err = fmt.Fprintln(output, "\nVersion: database --version\nCommand flags: database <command> --help")
		return true, err
	}
	if args[0] == "--version" {
		if len(args) != 1 {
			return true, fmt.Errorf("--version accepts no additional arguments")
		}
		info, _ := debug.ReadBuildInfo()
		encoder := json.NewEncoder(output)
		encoder.SetIndent("", "  ")
		return true, encoder.Encode(databaseVersionInformation(info))
	}
	for _, cmd := range databaseCommands {
		if args[0] == cmd {
			return false, nil
		}
	}
	return true, fmt.Errorf("Unknown command: %s", args[0])
}
