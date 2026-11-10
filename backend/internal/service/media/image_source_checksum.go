package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
)

// Legacy records may predate upload checksum enforcement. An explicit ensure
// can pin their existing bytes without retransmission; dry-run calls this on a
// copy and does not persist anything. Known metadata checksums must still match.
func (s *MediaService) storedImageChecksum(ctx context.Context, a *m.MediaAsset) (string, error) {
	options := s.imageProcessingPolicy().options
	if err := options.validate(); err != nil {
		return "", err
	}
	object, err := s.manager.Get(ctx, a.Driver, driver.GetObjectInput{Bucket: a.Bucket, ObjectKey: a.StorageKey})
	if err != nil {
		return "", err
	}
	if object == nil || object.Body == nil {
		return "", driver.ErrNotFound
	}
	defer object.Body.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(object.Body, options.MaxSourceBytes+1))
	if err != nil {
		return "", err
	}
	if n > options.MaxSourceBytes {
		return "", &imageRenderError{"MEDIA_IMAGE_BYTE_LIMIT"}
	}
	if n == 0 {
		return "", driver.ErrNotFound
	}
	checksum := hex.EncodeToString(hash.Sum(nil))
	known, err := contentSHA256FromMetadata(extractMeta(a.Meta))
	if err != nil || known != "" && known != checksum {
		return "", &imageRenderError{"MEDIA_IMAGE_SOURCE_CHECKSUM_MISMATCH"}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return checksum, nil
}

func sourceBackfillError(err error) string {
	var render *imageRenderError
	if errors.As(err, &render) {
		return render.code
	}
	return "MEDIA_BACKFILL_STORAGE_UNAVAILABLE"
}
