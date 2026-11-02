package archive_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

type legacyManifest struct {
	Version    int     `json:"version"`
	VerifiedAt float64 `json:"verified_at"`
	Snapshot   struct {
		Source     string `json:"source"`
		Name       string `json:"name"`
		Size       int64  `json:"size"`
		ModifiedNS int64  `json:"mtime_ns"`
		ID         string `json:"id"`
	} `json:"snapshot"`
	Object struct {
		Bucket  string `json:"bucket"`
		Key     string `json:"key"`
		Version string `json:"version_id"`
		SHA256  string `json:"sha256"`
		Bytes   int64  `json:"bytes"`
	} `json:"object"`
}
type LegacyImportResult struct {
	Imported    int `json:"imported"`
	Existing    int `json:"existing"`
	Failed      int `json:"failed"`
	Unconfirmed int `json:"unconfirmed"`
}

func (s *Service) decodeLegacyJob(j jobRecord, raw []byte) (Job, error) {
	var m legacyManifest
	if json.Unmarshal(raw, &m) != nil || m.Version != 1 || j.Target == nil || len(j.Files) != 1 {
		return Job{}, errors.New("历史归档清单格式不匹配")
	}
	f := j.Files[0]
	if m.Snapshot.Name != j.SourceFileName || m.Snapshot.Size != f.Size || m.Object.Bytes != f.Size || m.Object.SHA256 != f.SHA256 || m.Object.Key != f.ArchiveKey || m.Object.Version != f.ArchiveVersion || m.Object.Bucket != j.Target.Bucket {
		return Job{}, errors.New("历史归档内容或来源标识变化")
	}
	return j.Job, nil
}
func (s *Service) legacyReceiptVerified(j jobRecord) bool {
	if s.cfg.LegacyReceiptDirectory == "" || !filepath.IsAbs(s.cfg.LegacyReceiptDirectory) {
		return false
	}
	entries, err := os.ReadDir(s.cfg.LegacyReceiptDirectory)
	if err != nil || len(entries) > 10000 {
		return false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p := filepath.Join(s.cfg.LegacyReceiptDirectory, e.Name())
		st, err := os.Lstat(p)
		if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
			continue
		}
		var r struct {
			State  string `json:"state"`
			Object struct {
				Key             string `json:"key"`
				Version         string `json:"version_id"`
				SHA256          string `json:"sha256"`
				ManifestKey     string `json:"manifest_key"`
				ManifestVersion string `json:"manifest_version_id"`
				ManifestSHA256  string `json:"manifest_sha256"`
			} `json:"object"`
		}
		if readJSON(p, &r) != nil || r.State != "deleted" {
			continue
		}
		f := j.Files[0]
		if r.Object.Key == f.ArchiveKey && r.Object.Version == f.ArchiveVersion && r.Object.SHA256 == f.SHA256 && r.Object.ManifestKey == j.RemotePath && r.Object.ManifestVersion == j.RemoteManifestVersion && r.Object.ManifestSHA256 == j.RemoteManifestSHA256 {
			return true
		}
	}
	return false
}

// ImportLegacy 只导入明确选定目标中的历史清单；不启用归档、不删除任何对象。
func (s *Service) ImportLegacy(ctx context.Context, targetID string) (LegacyImportResult, error) {
	var out LegacyImportResult
	if err := s.check(ctx); err != nil {
		return out, err
	}
	t, err := s.target(targetID)
	if err != nil {
		return out, err
	}
	if t.Driver != "s3" || !s.targetVerified(t) {
		return out, failure(422, "ARCHIVE_TARGET_UNVERIFIED", "请先验证要纳管的 S3 归档目标")
	}
	unlock, err := s.lock(ctx, "legacy-import")
	if err != nil {
		return out, ErrBusy
	}
	defer unlock()
	c, err := s.s3Client(t)
	if err != nil {
		return out, err
	}
	versions, err := listS3Versions(ctx, c, t, t.Prefix+"/")
	if err != nil {
		return out, err
	}
	for _, v := range versions {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		if !v.IsLatest || !strings.Contains(v.Key, "/manifests/") || !strings.HasSuffix(v.Key, ".json") || v.Size < 1 || v.Size > 4<<20 {
			continue
		}
		id := uuid.NewSHA1(uuid.NameSpaceURL, []byte(t.ID+"|"+v.Key+"|"+v.VersionID)).String()
		if existing, e := s.readJob(id); e == nil {
			if !existing.ProvenanceVerified && s.legacyReceiptVerified(existing) {
				existing.ProvenanceVerified = true
				_ = s.saveJob(&existing)
			}
			out.Existing++
			continue
		}
		if err = s.catalogCapacity(); err != nil {
			return out, err
		}
		f, e := c.GetObject(ctx, t.Bucket, v.Key, minio.GetObjectOptions{VersionID: v.VersionID})
		if e != nil {
			out.Failed++
			continue
		}
		st, e := f.Stat()
		raw, readErr := io.ReadAll(io.LimitReader(f, (4<<20)+1))
		f.Close()
		if e != nil || readErr != nil || st.VersionID != v.VersionID || int64(len(raw)) != v.Size {
			out.Failed++
			continue
		}
		if !strings.HasPrefix(v.Key, t.Prefix+"/manifests/") {
			saved, e := s.recoverNativeManifest(ctx, c, t, v, raw)
			if e != nil {
				out.Failed++
			} else if saved {
				out.Imported++
				out.Unconfirmed++
			} else {
				out.Existing++
			}
			continue
		}
		var m legacyManifest
		if json.Unmarshal(raw, &m) != nil || m.Version != 1 || filepath.Base(m.Snapshot.Name) != m.Snapshot.Name || !cleanEntryPath(m.Snapshot.Name) || !shaPattern.MatchString(m.Object.SHA256) || m.Object.Bucket != t.Bucket || m.Object.Bytes != m.Snapshot.Size || m.Object.Key != t.Prefix+"/objects/"+m.Object.SHA256[:2]+"/"+m.Object.SHA256 || m.VerifiedAt <= 0 || m.VerifiedAt > float64(time.Now().Add(time.Minute).Unix()) || m.Object.Bytes < 0 || m.Snapshot.ModifiedNS <= 0 {
			out.Failed++
			continue
		}
		kind := "audit_logs"
		if m.Snapshot.Source == "postgres" || m.Snapshot.Source == "postgres-native" {
			kind = "postgresql_logs"
		} else if m.Snapshot.Source != "prod-audit" && m.Snapshot.Source != "dev-audit" {
			out.Failed++
			continue
		}
		if e = readS3Version(ctx, c, t, m.Object.Key, m.Object.Version, m.Object.Bytes, m.Object.SHA256, nil); e != nil {
			out.Failed++
			continue
		}
		sum := sha256.Sum256(raw)
		j := jobRecord{Job: Job{ID: id, PolicyID: uuid.NewSHA1(uuid.NameSpaceURL, []byte(t.ID+"|legacy|"+m.Snapshot.Source)).String(), SourceID: "legacy-" + m.Snapshot.Source, Kind: kind, Status: "success", Phase: "imported", Imported: true, SourceFileName: m.Snapshot.Name, CreatedAt: time.Unix(int64(m.VerifiedAt), 0).UTC(), RemoteState: "success", RemotePath: v.Key, RemoteManifestVersion: v.VersionID, RemoteManifestSHA256: hex.EncodeToString(sum[:]), Target: &t, Protected: true, FileCount: 1, SizeBytes: m.Object.Bytes, Warnings: []string{}, Files: []Entry{{Path: kind + "/" + m.Snapshot.Name, Size: m.Object.Bytes, SHA256: m.Object.SHA256, ModifiedAt: time.Unix(0, m.Snapshot.ModifiedNS), ArchiveKey: m.Object.Key, ArchiveVersion: m.Object.Version}}}}
		j.ProvenanceVerified = s.legacyReceiptVerified(j)
		if !j.ProvenanceVerified {
			j.Warnings = append(j.Warnings, "内容已验证；服务器回收凭据尚未匹配，保持保护且不纳入到期清理")
			out.Unconfirmed++
		}
		if e = s.saveJob(&j); e != nil {
			return out, e
		}
		out.Imported++
	}
	s.auditAction(ctx, "legacy_import", targetID, t.Revision)
	return out, nil
}
