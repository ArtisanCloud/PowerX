package archive_ops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/minio/minio-go/v7"
)

const MaxDatasetArchiveBytes = 256 << 10

// DatasetArchiveBinding is a published target identity pinned by Core when it
// accepts a dataset batch. It does not carry writable path or credentials.
type DatasetArchiveBinding struct {
	TargetID          string
	TargetRevision    int64
	TargetFingerprint string
	DatasetUUID       string
	TenantUUID        string
	BatchUUID         string
}

// DatasetArchiveObject identifies the exact immutable version verified by Core.
type DatasetArchiveObject struct {
	Version string
	SHA256  string
	Size    int64
}

func (s *Service) DatasetArchiveTarget(ctx context.Context, id string) (Target, error) {
	if s == nil || !validID(id) || !s.targetControlRequired() {
		return Target{}, failure(503, "DATASET_ARCHIVE_TARGET_UNAVAILABLE", "归档目标未就绪")
	}
	if err := s.available(); err != nil {
		return Target{}, err
	}
	if err := s.remoteExecutionAllowed(); err != nil {
		return Target{}, err
	}
	t, err := s.targetContext(ctx, id)
	if err != nil || t.Driver != "s3" || !s.targetVerified(t) {
		return Target{}, failure(409, "DATASET_ARCHIVE_TARGET_UNVERIFIED", "需要已校验的版本化对象存储目标")
	}
	return t, nil
}

func (s *Service) openDatasetArchiveTarget(ctx context.Context, b DatasetArchiveBinding) (Target, *minio.Client, string, error) {
	if !validID(b.DatasetUUID) || !validID(b.TenantUUID) || !validID(b.BatchUUID) {
		return Target{}, nil, "", failure(400, "DATASET_ARCHIVE_BINDING_INVALID", "归档范围无效")
	}
	t, err := s.DatasetArchiveTarget(ctx, b.TargetID)
	if err != nil {
		return Target{}, nil, "", err
	}
	if t.Revision != b.TargetRevision || t.VerifiedFingerprint != b.TargetFingerprint {
		return Target{}, nil, "", failure(409, "DATASET_ARCHIVE_TARGET_CHANGED", "归档目标已变更")
	}
	c, err := s.s3Client(t)
	if err != nil {
		return Target{}, nil, "", err
	}
	if err := checkPrivateVersionedBucket(ctx, c, t); err != nil {
		return Target{}, nil, "", err
	}
	// This namespace is disjoint from media/log policies and their retention.
	key := strings.TrimSuffix(t.Prefix, "/") + "/" + s.cfg.Instance + "/_plugin-datasets/" + b.DatasetUUID + "/" + b.TenantUUID + "/" + b.BatchUUID + ".jsonl"
	return t, c, key, nil
}

// PutDatasetArchive creates one fixed object and independently verifies its
// exact version on readback. An uncertain response can be retried with the
// same batch and content without creating another version.
func (s *Service) PutDatasetArchive(ctx context.Context, b DatasetArchiveBinding, data []byte) (DatasetArchiveObject, error) {
	if len(data) == 0 || len(data) > MaxDatasetArchiveBytes {
		return DatasetArchiveObject{}, failure(400, "DATASET_ARCHIVE_SIZE_INVALID", "归档对象大小超限")
	}
	t, c, key, err := s.openDatasetArchiveTarget(ctx, b)
	if err != nil {
		return DatasetArchiveObject{}, err
	}
	sum := sha256.Sum256(data)
	obj := DatasetArchiveObject{SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data))}
	put, err := c.PutObject(ctx, t.Bucket, key, bytes.NewReader(data), obj.Size,
		minio.PutObjectOptions{ContentType: "application/x-ndjson", OnlyIfAbsent: true})
	if err == nil {
		obj.Version = put.VersionID
	} else {
		// A conditional conflict or lost response may mean this batch committed.
		current, readErr := c.GetObject(ctx, t.Bucket, key, minio.GetObjectOptions{})
		if readErr != nil {
			return DatasetArchiveObject{}, failure(503, "DATASET_ARCHIVE_WRITE_UNCONFIRMED", "归档尚未确认")
		}
		defer current.Close()
		stat, readErr := current.Stat()
		if readErr != nil {
			return DatasetArchiveObject{}, failure(503, "DATASET_ARCHIVE_WRITE_UNCONFIRMED", "归档尚未确认")
		}
		obj.Version = stat.VersionID
	}
	if err := readS3Version(ctx, c, t, key, obj.Version, obj.Size, obj.SHA256, nil); err != nil {
		return DatasetArchiveObject{}, failure(409, "DATASET_ARCHIVE_VERIFICATION_FAILED", "归档版本或内容校验失败")
	}
	return obj, nil
}

// ReadDatasetArchive verifies the recorded version before returning any data.
func (s *Service) ReadDatasetArchive(ctx context.Context, b DatasetArchiveBinding, obj DatasetArchiveObject) ([]byte, error) {
	if obj.Size < 1 || obj.Size > MaxDatasetArchiveBytes {
		return nil, failure(409, "DATASET_ARCHIVE_VERIFICATION_FAILED", "无有效归档回执")
	}
	t, c, key, err := s.openDatasetArchiveTarget(ctx, b)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if err := readS3Version(ctx, c, t, key, obj.Version, obj.Size, obj.SHA256, &output); err != nil {
		return nil, failure(409, "DATASET_ARCHIVE_VERIFICATION_FAILED", "归档版本或内容校验失败")
	}
	return output.Bytes(), nil
}
