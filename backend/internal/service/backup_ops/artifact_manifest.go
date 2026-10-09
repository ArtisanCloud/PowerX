package backup_ops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/jackc/pgx/v5"
)

type artifactManifest struct {
	Contract  string    `json:"contract"`
	Database  string    `json:"source_database"`
	JobID     uint64    `json:"job_id"`
	PolicyID  uint64    `json:"policy_id"`
	File      string    `json:"file"`
	CreatedAt time.Time `json:"created_at"`
	Size      int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	Format    string    `json:"format"`
}

func writeArtifactSidecars(path, dsn string, job *modelops.BackupJob, artifact *modelops.BackupArtifact) error {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("backup source metadata unavailable")
	}
	manifest := artifactManifest{Contract: "powerx.database-backup/v1", Database: cfg.Database, JobID: job.ID, PolicyID: job.PolicyID, File: filepath.Base(path), CreatedAt: time.Now().UTC(), Size: artifact.SizeBytes, SHA256: artifact.Checksum, Format: "postgresql-custom"}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	for _, file := range []struct {
		suffix string
		data   []byte
	}{{".json", append(data, '\n')}, {".sha256", []byte(artifact.Checksum + "  " + filepath.Base(path) + "\n")}} {
		tmp, err := os.CreateTemp(filepath.Dir(path), ".manifest-")
		if err != nil {
			return err
		}
		name := tmp.Name()
		_, err = tmp.Write(file.data)
		if err == nil {
			err = tmp.Sync()
		}
		closeErr := tmp.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(name, path+file.suffix)
		}
		if err != nil {
			os.Remove(name)
			return err
		}
	}
	return nil
}

// 校验同名前缀的清单后再清理，旧备份没有 sidecar 时仍可按已登记元数据验证。
func removeArtifactFiles(path string, artifact *modelops.BackupArtifact) error {
	var sidecars []string
	for _, suffix := range []string{".json", ".sha256"} {
		file := path + suffix
		info, err := os.Lstat(file)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > 65536 {
			return fmt.Errorf("unsafe backup sidecar: %s", file)
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if suffix == ".sha256" {
			if strings.TrimSpace(string(data)) != artifact.Checksum+"  "+filepath.Base(path) {
				return fmt.Errorf("backup sidecar checksum mismatch")
			}
		} else {
			var manifest artifactManifest
			if json.Unmarshal(data, &manifest) != nil || manifest.Contract != "powerx.database-backup/v1" || manifest.File != filepath.Base(path) || manifest.SHA256 != artifact.Checksum || manifest.JobID != artifact.JobID {
				return fmt.Errorf("backup manifest mismatch")
			}
		}
		sidecars = append(sidecars, file)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, file := range sidecars {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
