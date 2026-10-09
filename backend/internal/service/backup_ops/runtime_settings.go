package backup_ops

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sys/unix"
)

type RuntimeSettings struct {
	SourceHost        string   `json:"source_host"`
	SourcePort        uint16   `json:"source_port"`
	SourceDatabase    string   `json:"source_database"`
	ArtifactDirectory string   `json:"artifact_directory"`
	PathTemplate      string   `json:"path_template"`
	Format            string   `json:"format"`
	Scope             string   `json:"scope"`
	RestoreMode       string   `json:"restore_mode"`
	Ready             bool     `json:"ready"`
	Problems          []string `json:"problems"`
}

// RuntimeSettings 仅提供管理员操作所需信息，禁止返回 DSN 和密码。
func (s *JobService) RuntimeSettings() RuntimeSettings {
	out := RuntimeSettings{ArtifactDirectory: s.artifactBaseDir, PathTemplate: "<tenant>/policy_<id>/<YYYY>/<MM>/<DD>/job_<id>_<UTC>.dump", Format: "PostgreSQL custom archive (.dump) + .dump.sha256 + .dump.json", Scope: "all_database_schemas", RestoreMode: "new_isolated_database", Problems: []string{}}
	cfg, err := pgx.ParseConfig(sourceDSN(s.db))
	if err != nil || strings.TrimSpace(sourceDSN(s.db)) == "" {
		out.Problems = append(out.Problems, "source database connection is unavailable")
	} else {
		out.SourceHost, out.SourcePort, out.SourceDatabase = cfg.Host, cfg.Port, cfg.Database
	}
	if s.artifactBaseDirErr != nil {
		out.Problems = append(out.Problems, s.artifactBaseDirErr.Error())
	}
	parent := s.artifactBaseDir
	for parent != "" {
		_, e := os.Stat(parent)
		if e == nil {
			break
		}
		next := filepath.Dir(parent)
		if next == parent {
			parent = ""
			break
		}
		parent = next
	}
	if parent == "" || unix.Access(parent, unix.W_OK) != nil {
		out.Problems = append(out.Problems, "backup directory is not writable")
	}
	for _, tool := range []string{"pg_dump", "pg_restore", "psql", "createdb", "dropdb"} {
		if _, e := exec.LookPath(tool); e != nil {
			out.Problems = append(out.Problems, fmt.Sprintf("%s is not available in PATH", tool))
		}
	}
	for _, script := range []string{"backup-db.sh", "restore-drill.sh"} {
		if info, e := os.Stat(filepath.Join(s.scriptDir, script)); e != nil || info.IsDir() {
			out.Problems = append(out.Problems, script+" is unavailable")
		}
	}
	out.Ready = len(out.Problems) == 0
	return out
}
