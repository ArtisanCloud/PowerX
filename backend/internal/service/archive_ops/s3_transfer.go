package archive_ops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/minio/minio-go/v7"
)

func (s *Service) s3Namespace(t Target, policy string) string {
	return t.Prefix + "/" + s.cfg.Instance + "/" + policy
}

func listS3Versions(ctx context.Context, c *minio.Client, t Target, prefix string) ([]minio.ObjectVersion, error) {
	if !strings.HasPrefix(prefix, t.Prefix+"/") {
		return nil, errors.New("归档范围不匹配")
	}
	out := []minio.ObjectVersion{}
	key, version := "", ""
	for page := 0; page < 100; page++ {
		p, err := c.ListObjectVersions(ctx, t.Bucket, prefix, key, version, 1000)
		if err != nil {
			return nil, errors.New("无法完整列出归档版本，保留现有对象")
		}
		for i := range p.DeleteMarkers {
			p.DeleteMarkers[i].DeleteMarker = true
		}
		for _, v := range append(p.Versions, p.DeleteMarkers...) {
			if !strings.HasPrefix(v.Key, prefix) || v.VersionID == "" || v.VersionID == "null" {
				return nil, errors.New("归档版本范围或标识异常")
			}
			out = append(out, v)
		}
		if !p.Truncated {
			return out, nil
		}
		if p.NextKeyMarker == "" || (p.NextKeyMarker == key && p.NextVersionIDMarker == version) {
			return nil, errors.New("归档版本分页不完整")
		}
		key, version = p.NextKeyMarker, p.NextVersionIDMarker
	}
	return nil, errors.New("归档版本清单超限，需要缩小管理范围")
}
func (s *Service) uploadS3Snapshot(ctx context.Context, j *jobRecord) error {
	if err := s.transferAllowed(ctx, *j); err != nil {
		return err
	}
	t := *j.Target
	c, err := s.s3Client(t)
	if err != nil {
		return err
	}
	if err = checkPrivateVersionedBucket(ctx, c, t); err != nil {
		return err
	}
	p, err := s.policy(ctx, j.PolicyID)
	if err != nil {
		return err
	}
	namespace := s.s3Namespace(t, j.PolicyID)
	versions, err := listS3Versions(ctx, c, t, namespace+"/")
	if err != nil {
		return err
	}
	var used int64
	manifests := map[string]bool{}
	for _, v := range versions {
		used += v.Size
		if strings.HasPrefix(v.Key, namespace+"/manifests/") {
			manifests[v.Key] = true
		}
	}
	latest := map[string]minio.ObjectVersion{}
	for _, v := range versions {
		if v.IsLatest && !v.DeleteMarker {
			latest[v.Key] = v
		}
	}
	if !manifests[namespace+"/manifests/"+j.ID+".json"] && len(manifests) >= p.RemoteRetentionCount {
		return errors.New("归档份数预算不足，未缩短保留期限，请处理已到期历史或调整预算")
	}
	var added int64
	seen := map[string]bool{}
	for _, entry := range j.Files {
		if !shaPattern.MatchString(entry.SHA256) || !cleanEntryPath(entry.Path) || entry.Size < 0 {
			return errors.New("归档文件清单无效")
		}
		key := namespace + "/objects/" + entry.SHA256[:2] + "/" + entry.SHA256
		if !seen[key] {
			if v, ok := latest[key]; !ok || v.Size != entry.Size {
				added += entry.Size
			}
			seen[key] = true
		}
	}
	if p.RemoteMaxBytes < 1 || used+added+(4<<20) > p.RemoteMaxBytes {
		return errors.New("归档容量预算不足，未缩短保留期限；请调整预算或处理到期对象")
	}
	for i := range j.Files {
		if err = s.transferAllowed(ctx, *j); err != nil {
			return err
		}
		entry := &j.Files[i]
		if !shaPattern.MatchString(entry.SHA256) || !cleanEntryPath(entry.Path) {
			return errors.New("归档文件清单无效")
		}
		if err = verifyObject(ctx, s.objectPath(entry.SHA256), *entry); err != nil {
			return err
		}
		entry.ArchiveKey = namespace + "/objects/" + entry.SHA256[:2] + "/" + entry.SHA256
		if v, ok := latest[entry.ArchiveKey]; ok {
			entry.ArchiveVersion = v.VersionID
		} else {
			f, err := os.Open(s.objectPath(entry.SHA256))
			if err != nil {
				return err
			}
			var info minio.UploadInfo
			e := s.guardTransfer(ctx, *j, func(run context.Context) error {
				var err error
				info, err = c.PutObject(run, t.Bucket, entry.ArchiveKey, f, entry.Size, minio.PutObjectOptions{ContentType: "application/octet-stream", OnlyIfAbsent: true})
				return err
			})
			f.Close()
			if e != nil {
				return archiveWriteError(e, "归档对象写入失败，源文件继续保留")
			}
			entry.ArchiveVersion = info.VersionID
			latest[entry.ArchiveKey] = minio.ObjectVersion{VersionID: info.VersionID, Size: entry.Size}
		}
		if err = readS3Version(ctx, c, t, entry.ArchiveKey, entry.ArchiveVersion, entry.Size, entry.SHA256, nil); err != nil {
			return err
		}
	}
	if err = s.transferAllowed(ctx, *j); err != nil {
		return err
	}
	raw, err := json.Marshal(archiveManifest(j.Job))
	if err != nil {
		return err
	}
	if len(raw) > 4<<20 {
		return errors.New("归档清单过大")
	}
	j.RemotePath = namespace + "/manifests/" + j.ID + ".json"
	var info minio.UploadInfo
	err = s.guardTransfer(ctx, *j, func(run context.Context) error {
		var putErr error
		info, putErr = c.PutObject(run, t.Bucket, j.RemotePath, bytes.NewReader(raw), int64(len(raw)), minio.PutObjectOptions{ContentType: "application/json"})
		return putErr
	})
	if err != nil {
		return archiveWriteError(err, "归档清单发布失败")
	}
	digest := sha256.Sum256(raw)
	j.RemoteManifestVersion = info.VersionID
	j.RemoteManifestSHA256 = hex.EncodeToString(digest[:])
	if err = readS3Version(ctx, c, t, j.RemotePath, j.RemoteManifestVersion, int64(len(raw)), j.RemoteManifestSHA256, nil); err != nil {
		return err
	}
	return s.transferAllowed(ctx, *j)
}

// 保留权威校验的安全错误码，外部存储的原始错误不得进入任务展示。
func archiveWriteError(err error, message string) error {
	var app *dto.AppError
	if errors.As(err, &app) && app.Code != "" {
		return failure(app.HTTPCode, app.Code, message+"（"+app.Code+"）")
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return failure(503, "ARCHIVE_WRITE_INTERRUPTED", message+"（执行超时或服务停止，源文件继续保留）")
	}
	return errors.New(message)
}

func (s *Service) readS3Job(ctx context.Context, j jobRecord) (Job, error) {
	var out Job
	if j.Target == nil || j.Target.Driver != "s3" || j.RemoteManifestVersion == "" || !shaPattern.MatchString(j.RemoteManifestSHA256) {
		return out, errors.New("归档清单缺少可核验的版本记录")
	}
	t := *j.Target
	c, err := s.s3Client(t)
	if err != nil {
		return out, err
	}
	f, err := c.GetObject(ctx, t.Bucket, j.RemotePath, minio.GetObjectOptions{VersionID: j.RemoteManifestVersion})
	if err != nil {
		return out, errors.New("远端清单不可读取")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.VersionID != j.RemoteManifestVersion || st.Size > 4<<20 {
		return out, errors.New("远端清单版本或大小无效")
	}
	raw, err := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	if err != nil || int64(len(raw)) != st.Size {
		return out, errors.New("远端清单读取不完整")
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != j.RemoteManifestSHA256 {
		return out, errors.New("远端清单内容校验失败")
	}
	if j.Imported {
		return s.decodeLegacyJob(j, raw)
	}
	if json.Unmarshal(raw, &out) != nil || out.ID != j.ID || out.PolicyID != j.PolicyID || out.SourceID != j.SourceID || len(out.Files) != len(j.Files) {
		return out, errors.New("远端清单身份不匹配")
	}
	for i, want := range j.Files {
		got := out.Files[i]
		if got.Path != want.Path || got.Size != want.Size || got.SHA256 != want.SHA256 || got.ArchiveKey != want.ArchiveKey || got.ArchiveVersion != want.ArchiveVersion {
			return out, errors.New("远端文件清单发生变化")
		}
	}
	return out, nil
}
func (s *Service) verifyS3Job(ctx context.Context, j jobRecord) (Job, error) {
	out, err := s.readS3Job(ctx, j)
	if err != nil {
		return out, err
	}
	c, err := s.s3Client(*j.Target)
	if err != nil {
		return out, err
	}
	if err = checkPrivateVersionedBucket(ctx, c, *j.Target); err != nil {
		return out, err
	}
	for _, f := range out.Files {
		if err = readS3Version(ctx, c, *j.Target, f.ArchiveKey, f.ArchiveVersion, f.Size, f.SHA256, nil); err != nil {
			return out, err
		}
	}
	return out, nil
}
func (s *Service) ensureS3Objects(ctx context.Context, j jobRecord) error {
	remote, err := s.readS3Job(ctx, j)
	if err != nil {
		return err
	}
	c, err := s.s3Client(*j.Target)
	if err != nil {
		return err
	}
	var peak int64
	for _, entry := range remote.Files {
		if verifyObject(ctx, s.objectPath(entry.SHA256), entry) != nil {
			peak += entry.Size
		}
	}
	ctx, release, err := budget.BeginPath(ctx, s.cfg.Directory, "archive_download", peak+(1<<20))
	if err != nil {
		return err
	}
	defer release()
	for _, entry := range remote.Files {
		dest := s.objectPath(entry.SHA256)
		if verifyObject(ctx, dest, entry) == nil {
			continue
		}
		if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return err
		}
		f, err := os.CreateTemp(filepath.Dir(dest), ".fetch-")
		if err != nil {
			return err
		}
		name := f.Name()
		err = readS3Version(ctx, c, *j.Target, entry.ArchiveKey, entry.ArchiveVersion, entry.Size, entry.SHA256, budget.Writer(ctx, f))
		if err == nil {
			err = f.Sync()
		}
		closed := f.Close()
		if err == nil {
			err = closed
		}
		if err == nil {
			err = os.Rename(name, dest)
		}
		_ = os.Remove(name)
		if err != nil {
			return err
		}
	}
	return nil
}
