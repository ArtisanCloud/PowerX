package archive_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/minio/minio-go/v7"
	"strings"
	"time"
)

func (s *Service) recoverNativeManifest(ctx context.Context, c *minio.Client, t Target, v minio.ObjectVersion, raw []byte) (bool, error) {
	relative := strings.TrimPrefix(v.Key, t.Prefix+"/")
	parts := strings.Split(relative, "/")
	if len(parts) != 4 || !labelPattern.MatchString(parts[0]) || !validID(parts[1]) || parts[2] != "manifests" {
		return false, nil
	}
	var original Job
	if json.Unmarshal(raw, &original) != nil || !validID(original.ID) || original.PolicyID != parts[1] || parts[3] != original.ID+".json" || !isLogSource(original.Kind) || original.CreatedAt.IsZero() || original.CreatedAt.After(time.Now().Add(time.Minute)) || len(original.Files) == 0 || len(original.Files) > 10000 {
		return false, errors.New("native manifest identity invalid")
	}
	if old, e := s.readJob(original.ID); e == nil {
		if old.RemotePath != v.Key || old.RemoteManifestVersion != v.VersionID {
			return false, errors.New("catalog identity already bound to a different manifest")
		}
		return false, nil
	}
	ns := t.Prefix + "/" + parts[0] + "/" + parts[1]
	var size int64
	for _, f := range original.Files {
		if !shaPattern.MatchString(f.SHA256) || !cleanEntryPath(f.Path) || f.Size < 0 || f.ArchiveKey != ns+"/objects/"+f.SHA256[:2]+"/"+f.SHA256 {
			return false, errors.New("native manifest file boundary invalid")
		}
		if err := readS3Version(ctx, c, t, f.ArchiveKey, f.ArchiveVersion, f.Size, f.SHA256, nil); err != nil {
			return false, err
		}
		size += f.Size
		if size < 0 {
			return false, errors.New("native manifest size invalid")
		}
	}
	digest := sha256.Sum256(raw)
	original.Target = &t
	original.RemotePath = v.Key
	original.RemoteManifestVersion = v.VersionID
	original.RemoteManifestSHA256 = hex.EncodeToString(digest[:])
	original.Status = "pruned"
	original.RemoteState = "success"
	original.Phase = "catalog_rebuilt"
	original.Imported = false
	original.Recovered = true
	original.Protected = true
	original.ProvenanceVerified = false
	original.SizeBytes = size
	original.FileCount = len(original.Files)
	original.Error = ""
	original.RemoteError = ""
	original.Warnings = []string{"从远端版本重建目录；内容已验证，原策略及源端处置未作推断，保持保护"}
	return true, s.saveJob(&jobRecord{Job: original})
}
