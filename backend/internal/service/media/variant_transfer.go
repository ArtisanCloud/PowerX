package media

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"io"
	"strconv"
	"strings"
	"time"
)

type VariantTransferTicket struct {
	AssetUUID     string
	VariantUUID   string
	Action        string
	ExpiresAt     time.Time
	Version       uint64
	ParentVersion uint64
	Token         string
	MimeType      string
	SizeBytes     int64
}

func (s *MediaService) variantSignature(asset, variant, action string, version, parent uint64, exp int64) string {
	mac := hmac.New(sha256.New, s.publicResourceTokenSecret)
	fmt.Fprintf(mac, "variant\n%s\n%s\n%s\n%d\n%d\n%d", asset, variant, action, version, parent, exp)
	return hex.EncodeToString(mac.Sum(nil))
}

func variantError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrAssetNotFound
	}
	return err
}

func (s *MediaService) IssueVariantTransferTicket(ctx context.Context, tenant, asset, variant, action string, ttl time.Duration) (*VariantTransferTicket, error) {
	if action != "upload" && action != "download" {
		return nil, ErrInvalidUploadMethod
	}
	if ttl < time.Minute || ttl > time.Hour {
		return nil, ErrInvalidStatusTransition
	}
	if len(s.publicResourceTokenSecret) == 0 {
		return nil, fmt.Errorf("media.ticket_secret_missing")
	}
	var ticket *VariantTransferTicket
	err := s.repo.WithVariantTransfer(ctx, tenant, asset, variant, func(p *m.MediaAsset, v *m.MediaAssetVariant) error {
		if p.UploadState == m.UploadStateDeleted {
			return ErrAssetNotFound
		}
		if action == "upload" && (v.UploadState != m.UploadStatePending || v.UploadExpiresAt == nil || !time.Now().Before(*v.UploadExpiresAt)) {
			return ErrInvalidStatusTransition
		}
		if action == "download" && v.UploadState != m.UploadStateReady {
			return ErrInvalidStatusTransition
		}
		expires := time.Now().Add(ttl).UTC()
		if action == "upload" && expires.After(*v.UploadExpiresAt) {
			expires = *v.UploadExpiresAt
		}
		ticket = &VariantTransferTicket{AssetUUID: asset, VariantUUID: variant, Action: action, ExpiresAt: expires, Version: v.TicketVersion, ParentVersion: p.UploadTicketVersion, MimeType: v.MimeType, SizeBytes: v.SizeBytes}
		ticket.Token = s.variantSignature(asset, variant, action, v.TicketVersion, p.UploadTicketVersion, expires.Unix())
		return nil
	})
	return ticket, variantError(err)
}

// Signature is checked before the only global UUID lookup. A domain-separated
// MAC binds parent, child, operation, both revocation versions and expiry.
func (s *MediaService) ConsumeVariantTransfer(ctx context.Context, asset, variant, action, expRaw, versionRaw, parentRaw, token string, body io.Reader, size int64, mime string) (*driver.GetObjectResult, error) {
	for _, id := range []string{asset, variant} {
		parsed, e := uuid.Parse(id)
		if e != nil || parsed == uuid.Nil || parsed.String() != id {
			return nil, ErrAssetNotFound
		}
	}
	exp, e := strconv.ParseInt(expRaw, 10, 64)
	if e != nil || exp <= time.Now().Unix() {
		return nil, ErrUploadTicketExpired
	}
	version, e := strconv.ParseUint(versionRaw, 10, 64)
	if e != nil {
		return nil, ErrUploadTicketExpired
	}
	parent, e := strconv.ParseUint(parentRaw, 10, 64)
	if e != nil {
		return nil, ErrUploadTicketExpired
	}
	if len(s.publicResourceTokenSecret) == 0 || (action != "upload" && action != "download") || !hmac.Equal([]byte(token), []byte(s.variantSignature(asset, variant, action, version, parent, exp))) {
		return nil, ErrUploadTicketExpired
	}
	p, e := s.repo.FindByUUIDGlobal(ctx, asset, false)
	if e != nil {
		return nil, variantError(e)
	}
	if s.manager == nil {
		return nil, fmt.Errorf("media.manager_missing")
	}
	var result *driver.GetObjectResult
	var invalid bool
	var storageErr error
	e = s.repo.WithVariantTransfer(ctx, p.TenantUUID, asset, variant, func(p *m.MediaAsset, v *m.MediaAssetVariant) error {
		if exp <= time.Now().Unix() || p.UploadState == m.UploadStateDeleted || p.UploadTicketVersion != parent || v.TicketVersion != version {
			return ErrUploadTicketExpired
		}
		if action == "download" {
			if v.UploadState != m.UploadStateReady {
				return ErrInvalidStatusTransition
			}
			var err error
			result, err = s.manager.Get(ctx, v.Driver, driver.GetObjectInput{Bucket: v.Bucket, ObjectKey: v.StorageKey})
			if err == nil && (result == nil || result.Body == nil) {
				return fmt.Errorf("media.object_missing")
			}
			return err
		}
		if v.UploadState != m.UploadStatePending || v.UploadExpiresAt == nil || !time.Now().Before(*v.UploadExpiresAt) {
			return ErrUploadTicketExpired
		}
		if body == nil || size != v.SizeBytes || mime != v.MimeType {
			return ErrUploadValidationFailed
		}
		hash := sha256.New()
		limited := &io.LimitedReader{R: body, N: v.SizeBytes + 1}
		_, err := s.manager.Put(ctx, v.Driver, driver.PutObjectInput{Bucket: v.Bucket, ObjectKey: v.StorageKey, Body: io.TeeReader(limited, hash), Size: size, ContentType: mime, Overwrite: false})
		if err != nil {
			// A failed Put may have created a partial object. Never leave its
			// single-use ticket valid or silently retry/overwrite that object.
			storageErr = err
			v.UploadState = m.UploadStateFailed
			v.UploadExpiresAt = nil
			v.TicketVersion++
			return nil
		}
		if v.SizeBytes+1-limited.N != v.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != v.ExpectedChecksum {
			v.UploadState = m.UploadStateFailed
			v.UploadExpiresAt = nil
			invalid = true
		} else {
			v.UploadState = m.UploadStateUploaded
		}
		v.TicketVersion++
		return nil
	})
	if e == nil && storageErr != nil {
		e = storageErr
	}
	if e == nil && invalid {
		e = ErrUploadValidationFailed
	}
	if e != nil && result != nil {
		// Get opened a stream inside the transaction; commit can still fail.
		// The caller must never receive an uncommitted download or leak it.
		if result.Body != nil {
			_ = result.Body.Close()
		}
		result = nil
	}
	return result, variantError(e)
}

func (s *MediaService) CompleteVariantUpload(ctx context.Context, tenant, asset, variant, checksum string) (*AssetVariant, error) {
	normalized, e := contentSHA256FromMetadata(map[string]any{"content_sha256": checksum})
	if e != nil || normalized == "" {
		return nil, ErrContentSHA256Invalid
	}
	if s.manager == nil {
		return nil, fmt.Errorf("media.manager_missing")
	}
	var result *AssetVariant
	var invalid bool
	e = s.repo.WithVariantTransfer(ctx, tenant, asset, variant, func(p *m.MediaAsset, v *m.MediaAssetVariant) error {
		if p.UploadState == m.UploadStateDeleted {
			return ErrAssetNotFound
		}
		if normalized != v.ExpectedChecksum {
			return ErrUploadValidationFailed
		}
		if v.UploadState == m.UploadStateReady {
			result = toAssetVariant(v)
			return nil
		}
		if v.UploadState != m.UploadStateUploaded || v.UploadExpiresAt == nil || !time.Now().Before(*v.UploadExpiresAt) {
			return ErrInvalidStatusTransition
		}
		object, err := s.manager.Get(ctx, v.Driver, driver.GetObjectInput{Bucket: v.Bucket, ObjectKey: v.StorageKey})
		if err != nil {
			return err
		}
		if object == nil || object.Body == nil {
			return fmt.Errorf("media.object_missing")
		}
		defer object.Body.Close()
		hash := sha256.New()
		count, err := io.Copy(hash, io.LimitReader(object.Body, v.SizeBytes+1))
		if err != nil {
			return err
		}
		invalid = count != v.SizeBytes || object.Size != v.SizeBytes || !strings.EqualFold(object.ContentType, v.MimeType) || hex.EncodeToString(hash.Sum(nil)) != v.ExpectedChecksum
		v.TicketVersion++
		v.UploadExpiresAt = nil
		if invalid {
			v.UploadState = m.UploadStateFailed
		} else {
			now := time.Now().UTC()
			v.UploadState = m.UploadStateReady
			v.CompletedAt = &now
		}
		result = toAssetVariant(v)
		return nil
	})
	if e == nil && invalid {
		e = ErrUploadValidationFailed
	}
	if e == nil {
		s.emitAudit(ctx, tenant, "media.variant.complete_upload", variant, nil, map[string]any{"asset_uuid": asset})
	}
	return result, variantError(e)
}
