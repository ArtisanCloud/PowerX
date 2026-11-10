package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/crc32"
	"image"
	"io"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver"
	ef "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const pngChecksumRepairContract = "powerx.png_checksum_repair.v1"
const knownPNGCRCChecksum = "4b5c5c92cec3b23e6a294fc0eea43234ef5126c5a64f4c6c531ac8430ab0b844"

// PNGChecksumRepairPlan 只授权一个已确认样本的派生图，不修改原图或任务历史。
type PNGChecksumRepairPlan struct {
	Contract         string       `json:"contract"`
	PlanUUID         string       `json:"plan_uuid"`
	Environment      string       `json:"environment"`
	Tenant           string       `json:"tenant_uuid"`
	Asset            string       `json:"asset_uuid"`
	Before           m.MediaAsset `json:"before"`
	Fingerprint      string       `json:"fingerprint"`
	SourceSHA256     string       `json:"source_sha256"`
	RecoveredSHA256  string       `json:"recovered_sha256"`
	DerivativeSHA256 string       `json:"derivative_sha256"`
	DerivativeSize   int64        `json:"derivative_size"`
	Method           string       `json:"method"`
	CreatedAt        time.Time    `json:"created_at"`
	Seal             string       `json:"seal"`
}

func pngRepairBytesHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func pngRepairHash(value any) string { raw, _ := json.Marshal(value); return pngRepairBytesHash(raw) }

// 此函数不接入 renderSource。仅离线、显式修复已知 SHA 的 IDAT CRC。
func recoverKnownPNGCRC(raw []byte) ([]byte, error) {
	if len(raw) != 68 || pngRepairBytesHash(raw) != knownPNGCRCChecksum {
		return nil, errors.New("MEDIA_PNG_REPAIR_SOURCE_NOT_APPROVED")
	}
	fixed := append([]byte(nil), raw...)
	binary.BigEndian.PutUint32(fixed[52:56], crc32.ChecksumIEEE(fixed[37:52]))
	img, format, err := image.Decode(bytes.NewReader(fixed))
	if err != nil || format != "png" || img.Bounds() != image.Rect(0, 0, 1, 1) {
		return nil, errors.New("MEDIA_PNG_REPAIR_OUTPUT_INVALID")
	}
	return fixed, nil
}

// 使用只读快照扫描，执行时使用同租户父行锁；所有已存在标准变体均拒绝覆盖。
func (s *MediaService) inspectPNGChecksumRepair(ctx context.Context, tx *gorm.DB, tenant, asset string, lock bool) (PNGChecksumRepairPlan, []byte, error) {
	p := PNGChecksumRepairPlan{Contract: pngChecksumRepairContract, Tenant: tenant, Asset: asset, Method: "known_png_idat_crc_v1"}
	q := tx.Where("tenant_uuid=? AND uuid=?", tenant, asset)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE", Options: "NOWAIT"})
	}
	if err := q.First(&p.Before).Error; err != nil {
		return p, nil, errors.New("MEDIA_PNG_REPAIR_SOURCE_UNAVAILABLE")
	}
	a := &p.Before
	if a.UploadState != m.UploadStateReady || a.Driver != "local" || a.MimeType != "image/png" || a.ExpectedChecksum != knownPNGCRCChecksum || a.SizeBytes != 68 {
		return p, nil, errors.New("MEDIA_PNG_REPAIR_SOURCE_NOT_APPROVED")
	}
	ref := derivativeRef(a)
	var history ef.TaskHistory
	if ref.TaskID == "" || ref.Recipe != ImageDerivativeRecipe || tx.Where("task_id=? AND tenant_key=? AND subscriber_id=?", ref.TaskID, tenant, derivativeSubscriber).First(&history).Error != nil || history.Status != ef.TaskHistoryStatusFailed || history.ErrorMessage != "MEDIA_IMAGE_DECODE_FAILED" {
		return p, nil, errors.New("MEDIA_PNG_REPAIR_TASK_NOT_ELIGIBLE")
	}
	var count int64
	if err := tx.Model(&m.MediaAssetVariant{}).Where("tenant_uuid=? AND asset_uuid=? AND variant IN ?", tenant, asset, []string{"preview", "thumbnail", "thumbnail_64"}).Count(&count).Error; err != nil || count != 0 {
		return p, nil, errors.New("MEDIA_PNG_REPAIR_VARIANT_CONFLICT")
	}
	object, err := s.manager.Get(ctx, a.Driver, driver.GetObjectInput{Bucket: a.Bucket, ObjectKey: a.StorageKey})
	if err != nil || object == nil || object.Body == nil {
		return p, nil, errors.New("MEDIA_PNG_REPAIR_SOURCE_UNAVAILABLE")
	}
	raw, err := io.ReadAll(io.LimitReader(object.Body, 69))
	closeErr := object.Body.Close()
	if err != nil || closeErr != nil || len(raw) != 68 || object.Size != 68 || pngRepairBytesHash(raw) != a.ExpectedChecksum {
		return p, nil, errors.New("MEDIA_PNG_REPAIR_SOURCE_CHANGED")
	}
	if _, _, err := renderSource(ctx, bytes.NewReader(raw), a.ExpectedChecksum); err == nil {
		return p, nil, errors.New("MEDIA_PNG_REPAIR_SOURCE_NOT_DAMAGED")
	}
	fixed, err := recoverKnownPNGCRC(raw)
	if err != nil {
		return p, nil, err
	}
	img, format, err := renderSource(ctx, bytes.NewReader(fixed), pngRepairBytesHash(fixed))
	if err != nil {
		return p, nil, err
	}
	body, mime, w, h, err := encodeDerivative(ctx, img, format, 64)
	if err != nil || mime != "image/png" || w != 1 || h != 1 || len(body) > Thumbnail64MaxBytes {
		return p, nil, errors.New("MEDIA_PNG_REPAIR_OUTPUT_INVALID")
	}
	p.Fingerprint = pngRepairHash(struct {
		Row             m.MediaAsset
		State, Checksum string
		Version         uint64
		Expires         *time.Time
		History         ef.TaskHistory
	}{*a, a.UploadState, a.ExpectedChecksum, a.UploadTicketVersion, a.UploadExpiresAt, history})
	p.SourceSHA256, p.RecoveredSHA256, p.DerivativeSHA256, p.DerivativeSize = a.ExpectedChecksum, pngRepairBytesHash(fixed), pngRepairBytesHash(body), int64(len(body))
	return p, body, nil
}

// ScanPNGChecksumRepair 不启动 TaskBus、迁移、插件或 HTTP 服务。
func (s *MediaService) ScanPNGChecksumRepair(ctx context.Context, tenant, asset, environment string) (PNGChecksumRepairPlan, error) {
	var p PNGChecksumRepairPlan
	if s.DB == nil || s.manager == nil || !canonicalBackfillTenant(tenant) || !canonicalBackfillTenant(asset) || environment == "" {
		return p, errors.New("MEDIA_PNG_REPAIR_SCOPE_INVALID")
	}
	var options *sql.TxOptions
	if s.DB.Dialector.Name() == "postgres" {
		options = &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead}
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		p, _, err = s.inspectPNGChecksumRepair(ctx, tx, tenant, asset, false)
		return err
	}, options)
	if err != nil {
		return PNGChecksumRepairPlan{}, err
	}
	p.PlanUUID, p.Environment, p.CreatedAt = uuid.NewString(), environment, time.Now().UTC()
	p.Seal = pngRepairHash(p)
	return p, nil
}

// ApplyPNGChecksumRepair 生成真实对象后读回、严格解码，再登记三种标准派生图。
// 原图、父资产和历史失败任务不改写；提交不确定时保留新对象并返回恢复要求。
func (s *MediaService) ApplyPNGChecksumRepair(ctx context.Context, plan PNGChecksumRepairPlan, environment string) ([]*m.MediaAssetVariant, error) {
	p := plan
	seal := p.Seal
	p.Seal = ""
	if p.Contract != pngChecksumRepairContract || seal != pngRepairHash(p) || p.Environment != environment || !canonicalBackfillTenant(p.PlanUUID) || !canonicalBackfillTenant(p.Tenant) || !canonicalBackfillTenant(p.Asset) {
		return nil, errors.New("MEDIA_PNG_REPAIR_PLAN_INVALID")
	}
	created := []*m.MediaAssetVariant{}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		current, body, err := s.inspectPNGChecksumRepair(ctx, tx, p.Tenant, p.Asset, true)
		if err != nil {
			return err
		}
		if current.Fingerprint != p.Fingerprint || current.SourceSHA256 != p.SourceSHA256 || current.RecoveredSHA256 != p.RecoveredSHA256 || current.DerivativeSHA256 != p.DerivativeSHA256 || current.DerivativeSize != p.DerivativeSize || current.Method != p.Method {
			return errors.New("MEDIA_PNG_REPAIR_PLAN_CHANGED")
		}
		for _, profile := range imageProfiles {
			a := current.Before
			v := &m.MediaAssetVariant{TenantUUID: p.Tenant, AssetUUID: p.Asset, Variant: profile.Name, Name: profile.Name + ".png", Driver: a.Driver, Bucket: a.Bucket, BaseURL: a.BaseURL, StorageKey: p.Asset + "/maintenance-png-checksum/" + p.PlanUUID + "/" + profile.Name + ".png", SizeBytes: int64(len(body)), MimeType: "image/png", UploadState: m.UploadStateReady, ExpectedChecksum: pngRepairBytesHash(body), TicketVersion: 1, Meta: derivativeMeta(&a, pngRepairBytesHash(body), 1, 1)}
			v.UUID = uuid.New()
			now := time.Now().UTC()
			v.CompletedAt = &now
			meta := extractMeta(v.Meta)
			meta["recovery"] = map[string]any{"contract": p.Contract, "plan_uuid": p.PlanUUID, "method": p.Method, "recovered_sha256": p.RecoveredSHA256}
			v.Meta, _ = json.Marshal(meta)
			created = append(created, v)
			if _, err := s.manager.Put(ctx, v.Driver, driver.PutObjectInput{Bucket: v.Bucket, ObjectKey: v.StorageKey, Body: bytes.NewReader(body), Size: v.SizeBytes, ContentType: v.MimeType, Overwrite: false}); err != nil {
				return err
			}
			obj, err := s.manager.Get(ctx, v.Driver, driver.GetObjectInput{Bucket: v.Bucket, ObjectKey: v.StorageKey})
			if err != nil || obj == nil || obj.Body == nil {
				return errors.New("MEDIA_PNG_REPAIR_PERSIST_VALIDATION_FAILED")
			}
			stored, err := io.ReadAll(io.LimitReader(obj.Body, Thumbnail64MaxBytes+1))
			closeErr := obj.Body.Close()
			if err != nil || closeErr != nil || obj.Size != v.SizeBytes || !bytes.Equal(body, stored) {
				return errors.New("MEDIA_PNG_REPAIR_PERSIST_VALIDATION_FAILED")
			}
			if _, _, err := renderSource(ctx, bytes.NewReader(stored), v.ExpectedChecksum); err != nil {
				return err
			}
			if !variantRecordUsable(&a, v) {
				return errors.New("MEDIA_PNG_REPAIR_PERSIST_VALIDATION_FAILED")
			}
			if err := tx.Create(v).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil && len(created) > 0 {
		return created, errors.New("MEDIA_PNG_REPAIR_RECOVERY_REQUIRED")
	}
	if err == nil {
		s.publishDerivative(ctx, p.Tenant, p.Asset)
	}
	return created, err
}
