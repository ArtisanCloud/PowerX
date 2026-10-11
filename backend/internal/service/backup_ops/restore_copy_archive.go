package backup_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	repoops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/ops"
	"github.com/google/uuid"
)

const RestoreUploadLimit int64 = 512 << 20

type RestoreSource struct {
	SourceJobID uint64 `json:"source_job_id,omitempty"`
	ImportID    string `json:"import_id,omitempty"`
}

type restoreArchive struct {
	Path     string
	Name     string
	Checksum string
	Size     int64
	PolicyID uint64
}

type RestoreCopyPlan struct {
	Host                         string   `json:"host"`
	Port                         uint16   `json:"port"`
	CurrentDatabase              string   `json:"current_database"`
	TargetPattern                string   `json:"target_pattern"`
	ArchiveName                  string   `json:"archive_name"`
	ArchiveSHA256                string   `json:"archive_sha256"`
	ArchiveSize                  int64    `json:"archive_size"`
	ArchiveVersion               string   `json:"archive_version"`
	ServerVersion                string   `json:"server_version"`
	RequiresCompatibilityConsent bool     `json:"requires_compatibility_consent"`
	Extensions                   []string `json:"extensions"`
	Ready                        bool     `json:"ready"`
	Problems                     []string `json:"problems"`
}

type archiveHeader struct {
	version    string
	major      int
	tool       string
	extensions []string
}

var archiveVersionPattern = regexp.MustCompile(`Dumped from database version: ([0-9]+(?:\.[0-9]+)*)`)
var extensionTOCPattern = regexp.MustCompile(`EXTENSION - ([a-zA-Z0-9_-]+)\s`)

func readRestoreArchive(ctx context.Context, path string) (archiveHeader, error) {
	candidates := []string{}
	if current, err := exec.LookPath("pg_restore"); err == nil {
		candidates = append(candidates, current)
	}
	// macOS 的 Homebrew libpq 仅提供客户端；支持读取新版归档，不启动额外数据库服务。
	for _, path := range []string{"/opt/homebrew/opt/libpq/bin/pg_restore", "/usr/local/opt/libpq/bin/pg_restore"} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			candidates = append(candidates, path)
		}
	}
	for _, tool := range candidates {
		runCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		raw, err := exec.CommandContext(runCtx, tool, "--list", path).Output()
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return archiveHeader{}, ctx.Err()
			}
			continue
		}
		match := archiveVersionPattern.FindSubmatch(raw)
		if len(match) != 2 {
			continue
		}
		version := string(match[1])
		major, _ := strconv.Atoi(strings.Split(version, ".")[0])
		header := archiveHeader{version: version, major: major, tool: tool, extensions: []string{}}
		seen := map[string]bool{}
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.Contains(line, "COMMENT - EXTENSION") {
				continue
			}
			m := extensionTOCPattern.FindStringSubmatch(line)
			if len(m) == 2 && !seen[m[1]] {
				seen[m[1]] = true
				header.extensions = append(header.extensions, m[1])
			}
		}
		return header, nil
	}
	return archiveHeader{}, restoreFailure(422, "RESTORE_ARCHIVE_UNSUPPORTED", "无法读取归档；请确认文件格式和 pg_restore 客户端版本")
}

func (s *RestoreCopyService) archive(ctx context.Context, source RestoreSource) (restoreArchive, error) {
	if (source.SourceJobID == 0) == (source.ImportID == "") {
		return restoreArchive{}, restoreFailure(400, "RESTORE_SOURCE_INVALID", "只能选择一个备份作业或上传归档")
	}
	if source.ImportID != "" {
		if !validCopyID(source.ImportID) {
			return restoreArchive{}, restoreFailure(400, "RESTORE_IMPORT_INVALID", "上传归档标识无效")
		}
		path := filepath.Join(s.store.root, "imports", source.ImportID+".json")
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			return restoreArchive{}, restoreFailure(404, "RESTORE_IMPORT_NOT_FOUND", "上传归档不存在")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return restoreArchive{}, err
		}
		var metadata importLedger
		if json.Unmarshal(raw, &metadata) != nil || metadata.ID != source.ImportID || metadata.Binding != s.sourceKey {
			return restoreArchive{}, restoreFailure(404, "RESTORE_IMPORT_NOT_FOUND", "上传归档不属于当前实例")
		}
		file := filepath.Join(s.store.root, "imports", source.ImportID+".dump")
		artifact := &modelops.BackupArtifact{StorageURI: (&url.URL{Scheme: "file", Path: file}).String(), SizeBytes: metadata.SizeBytes, Checksum: metadata.SHA256}
		file, err = validatedArtifactPath(filepath.Join(s.store.root, "imports"), artifact)
		if err != nil {
			return restoreArchive{}, restoreFailure(422, "RESTORE_ARTIFACT_UNAVAILABLE", "上传归档文件不可用")
		}
		return restoreArchive{Path: file, Name: metadata.Name, Checksum: metadata.SHA256, Size: metadata.SizeBytes}, nil
	}
	job, err := repoops.NewBackupJobRepository(s.db).GetById(ctx, source.SourceJobID, nil)
	if err != nil {
		return restoreArchive{}, err
	}
	if job == nil || job.Status != modelops.BackupJobStatusSuccess {
		return restoreArchive{}, restoreFailure(400, "RESTORE_SOURCE_INVALID", "只能恢复成功的备份作业")
	}
	artifact, err := repoops.NewBackupArtifactRepository(s.db).GetLatestByJobID(ctx, job.ID)
	if err != nil {
		return restoreArchive{}, err
	}
	root, err := resolveBackupArtifactBaseDir()
	if err != nil {
		return restoreArchive{}, err
	}
	path, err := validatedArtifactPath(root, artifact)
	if err != nil {
		return restoreArchive{}, restoreFailure(422, "RESTORE_ARTIFACT_UNAVAILABLE", "备份文件不在当前实例的备份目录；请上传实际 dump 文件")
	}
	return restoreArchive{Path: path, Name: filepath.Base(path), Checksum: artifact.Checksum, Size: artifact.SizeBytes, PolicyID: job.PolicyID}, nil
}

func (s *RestoreCopyService) Import(ctx context.Context, name string, input io.Reader) (RestoreImport, error) {
	if err := s.authorize(ctx); err != nil {
		return RestoreImport{}, err
	}
	name = filepath.Base(name)
	if len(name) > 255 || !strings.EqualFold(filepath.Ext(name), ".dump") {
		return RestoreImport{}, restoreFailure(400, "RESTORE_IMPORT_INVALID", "请选择 PostgreSQL .dump 文件")
	}
	id := uuid.NewString()
	path := filepath.Join(s.store.root, "imports", id+".dump")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return RestoreImport{}, err
	}
	retained := false
	defer func() {
		f.Close()
		if !retained {
			os.Remove(path)
		}
	}()
	hash := sha256.New()
	reader := &restoreContextReader{ctx: ctx, reader: io.LimitReader(input, RestoreUploadLimit+1)}
	size, err := io.Copy(io.MultiWriter(f, hash), reader)
	if err != nil {
		return RestoreImport{}, err
	}
	if size <= 0 || size > RestoreUploadLimit {
		return RestoreImport{}, restoreFailure(413, "RESTORE_IMPORT_SIZE", "归档为空或超过 512 MiB 上传上限")
	}
	if err = f.Sync(); err != nil {
		return RestoreImport{}, err
	}
	if err = f.Close(); err != nil {
		return RestoreImport{}, err
	}
	if _, err = readRestoreArchive(ctx, path); err != nil {
		return RestoreImport{}, err
	}
	metadata := RestoreImport{ID: id, Name: name, SizeBytes: size, SHA256: hex.EncodeToString(hash.Sum(nil)), CreatedAt: time.Now().UTC()}
	raw, _ := json.Marshal(importLedger{RestoreImport: metadata, Binding: s.sourceKey})
	if err = atomicRestoreJSON(filepath.Join(s.store.root, "imports", id+".json"), raw); err != nil {
		return RestoreImport{}, err
	}
	retained = true
	return metadata, nil
}

type restoreContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *restoreContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func (s *RestoreCopyService) Plan(ctx context.Context, source RestoreSource) (RestoreCopyPlan, error) {
	if err := s.authorize(ctx); err != nil {
		return RestoreCopyPlan{}, err
	}
	archive, err := s.archive(ctx, source)
	if err != nil {
		return RestoreCopyPlan{}, err
	}
	header, err := readRestoreArchive(ctx, archive.Path)
	if err != nil {
		return RestoreCopyPlan{}, err
	}
	plan := RestoreCopyPlan{Host: s.cfg.Host, Port: s.cfg.Port, CurrentDatabase: s.cfg.Database, TargetPattern: "powerx_restore_copy_<UUID>", ArchiveName: archive.Name, ArchiveSHA256: archive.Checksum, ArchiveSize: archive.Size, ArchiveVersion: header.version, Extensions: header.extensions, Problems: []string{}}
	admin, err := s.connect(ctx, s.cfg.Database, s.cfg.User, s.cfg.Password)
	if err != nil {
		return plan, restoreFailure(503, "RESTORE_DATABASE_UNAVAILABLE", "当前 PostgreSQL 连接不可用")
	}
	defer admin.Close(ctx)
	var super, createDB, createRole bool
	var actual string
	err = admin.QueryRow(ctx, "SELECT current_database(),current_setting('server_version'),rolsuper,rolcreatedb,rolcreaterole FROM pg_roles WHERE rolname=current_user").Scan(&actual, &plan.ServerVersion, &super, &createDB, &createRole)
	if err != nil {
		return plan, err
	}
	if actual != s.cfg.Database {
		return plan, restoreFailure(409, "RESTORE_SOURCE_CHANGED", "运行中的数据库与恢复管理连接不一致")
	}
	if !super && (!createDB || !createRole) {
		plan.Problems = append(plan.Problems, "当前数据库账号需要 CREATEDB 和 CREATEROLE 权限")
	}
	serverMajor, _ := strconv.Atoi(strings.Split(plan.ServerVersion, ".")[0])
	plan.RequiresCompatibilityConsent = serverMajor < header.major
	for _, extension := range header.extensions {
		if !allowedRestoreExtension(extension) {
			plan.Problems = append(plan.Problems, fmt.Sprintf("不支持自动准备扩展 %s", extension))
			continue
		}
		var exists bool
		err = admin.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_available_extensions WHERE name=$1)", extension).Scan(&exists)
		if err != nil {
			return plan, err
		}
		if !exists {
			plan.Problems = append(plan.Problems, fmt.Sprintf("目标服务器缺少扩展 %s", extension))
		} else if !super {
			var needsSuper, trusted bool
			if err = admin.QueryRow(ctx, "SELECT v.superuser,v.trusted FROM pg_available_extension_versions v JOIN pg_available_extensions e ON e.name=v.name AND e.default_version=v.version WHERE v.name=$1", extension).Scan(&needsSuper, &trusted); err != nil {
				return plan, err
			}
			if needsSuper && !trusted {
				plan.Problems = append(plan.Problems, fmt.Sprintf("扩展 %s 需要数据库管理员准备权限", extension))
			}
		}
	}
	plan.Ready = len(plan.Problems) == 0
	return plan, nil
}

func allowedRestoreExtension(name string) bool {
	return map[string]bool{"vector": true, "pgcrypto": true, "uuid-ossp": true, "citext": true, "hstore": true}[name]
}
