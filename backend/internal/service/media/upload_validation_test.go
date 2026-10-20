package media

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/media/driver/local"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/media"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func zipFixture(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	for name, text := range parts {
		part, err := w.Create(name)
		require.NoError(t, err)
		_, err = io.WriteString(part, text)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return data.Bytes()
}

func officeFixture(t *testing.T, kind string) ([]byte, string) {
	t.Helper()
	var part, root, namespace, contentType, declared string
	switch kind {
	case "xlsx":
		part, root, namespace = "xl/workbook.xml", "workbook", "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
		declared = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		contentType = declared + ".main+xml"
	case "docx":
		part, root, namespace = "word/document.xml", "document", "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
		declared = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		contentType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	case "pptx":
		part, root, namespace = "ppt/presentation.xml", "presentation", "http://schemas.openxmlformats.org/presentationml/2006/main"
		declared = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
		contentType = declared + ".main+xml"
	}
	return zipFixture(t, map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/` + part + `" ContentType="` + contentType + `"/></Types>`,
		part:                  `<` + root + ` xmlns="` + namespace + `"></` + root + `>`,
	}), declared
}

func payloadHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func TestUploadedObjectRealFormatsAndForgery(t *testing.T) {
	xlsx, excelMIME := officeFixture(t, "xlsx")
	docx, wordMIME := officeFixture(t, "docx")
	pptx, slidesMIME := officeFixture(t, "pptx")
	ordinaryZIP := zipFixture(t, map[string]string{"readme.txt": "a normal archive"})
	var pngData, jpegData bytes.Buffer
	require.NoError(t, png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	require.NoError(t, jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 1, 1)), nil))
	for _, test := range []struct {
		name, mime, reason string
		data               []byte
	}{
		{"xlsx", excelMIME, "", xlsx}, {"docx", wordMIME, "", docx}, {"pptx", slidesMIME, "", pptx},
		{"markdown", "text/markdown", "", []byte("# 文档\n说明内容\n")},
		{"markdown_alias", "text/x-markdown; charset=utf-8", "", []byte("# document")},
		{"csv", "text/csv", "", []byte("name,value\na,1\n")},
		{"text_charset", "text/plain", "", []byte("plain text")},
		{"zip", "application/zip", "", ordinaryZIP},
		{"pdf", "application/pdf", "", []byte("%PDF-1.4\n%%EOF\n")},
		{"png", "image/png", "", pngData.Bytes()}, {"jpeg", "image/jpeg", "", jpegData.Bytes()},
		{"zip_as_excel", excelMIME, "invalid_office_package", ordinaryZIP},
		{"word_as_excel", excelMIME, "invalid_office_package", docx},
		{"image_as_excel", excelMIME, "invalid_office_package", pngData.Bytes()},
		{"image_as_markdown", "text/markdown", "mime_mismatch", pngData.Bytes()},
		{"excel_as_png", "image/png", "mime_mismatch", xlsx},
		{"truncated_excel", excelMIME, "invalid_office_package", xlsx[:len(xlsx)-30]},
	} {
		t.Run(test.name, func(t *testing.T) {
			digest := payloadHash(test.data)
			v, err := validateUploadedObject(bytes.NewReader(test.data), int64(len(test.data)), int64(len(test.data)), test.mime, digest, digest)
			require.NoError(t, err)
			require.Equal(t, test.reason, v.Reason)
		})
	}
	for _, test := range []struct {
		name     string
		size     int64
		checksum string
		reason   string
	}{
		{"wrong_size", int64(len(xlsx) - 1), payloadHash(xlsx), "size_mismatch"},
		{"wrong_hash", int64(len(xlsx)), strings.Repeat("a", 64), "checksum_mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			v, err := validateUploadedObject(bytes.NewReader(xlsx), int64(len(xlsx)), test.size, excelMIME, test.checksum, test.checksum)
			require.NoError(t, err)
			require.Equal(t, test.reason, v.Reason)
		})
	}
	oversizedXML := zipFixture(t, map[string]string{"[Content_Types].xml": strings.Repeat("x", maxContentTypesXML+1), "xl/workbook.xml": "<workbook/>"})
	v, err := validateUploadedObject(bytes.NewReader(oversizedXML), int64(len(oversizedXML)), int64(len(oversizedXML)), excelMIME, payloadHash(oversizedXML), payloadHash(oversizedXML))
	require.NoError(t, err)
	require.Equal(t, "invalid_office_package", v.Reason)
}

func TestUploadedObjectServerFileFixtures(t *testing.T) {
	for _, fixture := range []struct{ env, mime string }{
		{"POWERX_MEDIA_TEST_XLSX", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{"POWERX_MEDIA_TEST_MARKDOWN", "text/markdown"},
	} {
		t.Run(fixture.env, func(t *testing.T) {
			path := os.Getenv(fixture.env)
			if path == "" {
				t.Skip("optional readonly server file fixture")
			}
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			v, err := validateUploadedObject(bytes.NewReader(data), int64(len(data)), int64(len(data)), fixture.mime, payloadHash(data), payloadHash(data))
			require.NoError(t, err)
			require.Empty(t, v.Reason)
		})
	}
}

func newUploadFixture(t *testing.T) (*MediaService, *stubAssetRepo) {
	t.Helper()
	repo := newStubAssetRepo()
	driver, err := local.New(local.Options{BasePath: t.TempDir()})
	require.NoError(t, err)
	manager := mediamgr.New("local")
	manager.RegisterDriver(driver)
	svc := NewMediaService(nil, repo, manager, &stubAuditService{}, time.Hour)
	svc.publicResourceTokenSecret = []byte("media-retry-test-secret")
	return svc, repo
}

func authorizeUpload(t *testing.T, svc *MediaService, ticket *HostTransferTicket) *Asset {
	t.Helper()
	asset, err := svc.AuthorizeHostTransfer(context.Background(), ticket.AssetUUID, "upload", strconv.FormatInt(ticket.ExpiresAt.Unix(), 10), strconv.FormatUint(ticket.Version, 10), ticket.Token)
	require.NoError(t, err)
	return asset
}

func TestMediaFailedUploadRetryKeepsHistoryAndRevokesTickets(t *testing.T) {
	svc, repo := newUploadFixture(t)
	data := []byte("# retry the historical README\n")
	in := CreateAssetInput{TenantUUID: mediaTenantUUID, UploadMethod: UploadMethodPresign, Name: "README.md", MimeType: "text/markdown", SizeBytes: int64(len(data)), ContentSHA256: payloadHash(data)}
	first, err := svc.CreateAsset(context.Background(), in)
	require.NoError(t, err)
	oldTicket, err := svc.IssueHostTransferTicket(context.Background(), mediaTenantUUID, first.UUID, "upload", time.Minute)
	require.NoError(t, err)
	oldSnapshot := authorizeUpload(t, svc, oldTicket)
	// 模拟旧版本留下的失败状态；大小、哈希及原归属保持原值。
	row := repo.assets[first.UUID]
	row.UploadState, row.UploadExpiresAt = m.UploadStateFailed, nil
	row.UploadTicketVersion++
	row.Meta = datatypes.JSON(`{"custom":"kept","upload_failures":[{"reason":"mime_mismatch"}]}`)
	beforeVersion := row.UploadTicketVersion
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := svc.CreateAsset(context.Background(), in); errors <- e }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	require.Equal(t, beforeVersion+1, repo.assets[first.UUID].UploadTicketVersion)
	require.Equal(t, m.UploadStatePending, repo.assets[first.UUID].UploadState)
	require.Contains(t, string(repo.assets[first.UUID].Meta), "mime_mismatch")
	require.Contains(t, string(repo.assets[first.UUID].Meta), "kept")
	_, err = svc.AuthorizeHostTransfer(context.Background(), first.UUID, "upload", strconv.FormatInt(oldTicket.ExpiresAt.Unix(), 10), strconv.FormatUint(oldTicket.Version, 10), oldTicket.Token)
	require.ErrorIs(t, err, ErrUploadTicketExpired)
	err = svc.PutHostTransfer(context.Background(), oldSnapshot, bytes.NewReader(data), int64(len(data)), in.MimeType)
	require.ErrorIs(t, err, ErrUploadTicketExpired)
	ticket, err := svc.IssueHostTransferTicket(context.Background(), mediaTenantUUID, first.UUID, "upload", time.Minute)
	require.NoError(t, err)
	require.NoError(t, svc.PutHostTransfer(context.Background(), authorizeUpload(t, svc, ticket), bytes.NewReader(data), int64(len(data)), in.MimeType))
	ready, err := svc.CompleteAssetUpload(context.Background(), CompleteAssetUploadInput{TenantUUID: mediaTenantUUID, UUID: first.UUID, Checksum: payloadHash(data)})
	require.NoError(t, err)
	require.Equal(t, m.UploadStateReady, ready.UploadState)
	require.Contains(t, string(repo.assets[first.UUID].Meta), "mime_mismatch")
	version := repo.assets[first.UUID].UploadTicketVersion
	reused, err := svc.CreateAsset(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, ready.UUID, reused.UUID)
	require.Equal(t, version, repo.assets[first.UUID].UploadTicketVersion)
	_, err = svc.IssueHostTransferTicket(context.Background(), uuid.NewString(), first.UUID, "upload", time.Minute)
	require.ErrorIs(t, err, ErrAssetNotFound)
}

func TestMediaRetryRejectsChangedConstraintsAndExpiredUploadRecovers(t *testing.T) {
	svc, repo := newUploadFixture(t)
	in := CreateAssetInput{TenantUUID: mediaTenantUUID, UploadMethod: UploadMethodPresign, Name: "retry.md", MimeType: "text/markdown", SizeBytes: 8, ContentSHA256: strings.Repeat("a", 64)}
	asset, err := svc.CreateAsset(context.Background(), in)
	require.NoError(t, err)
	repo.assets[asset.UUID].UploadState = m.UploadStateFailed
	for _, change := range []func(*CreateAssetInput){func(i *CreateAssetInput) { i.SizeBytes++ }, func(i *CreateAssetInput) { i.MimeType = "image/png" }} {
		bad := in
		change(&bad)
		_, err := svc.CreateAsset(context.Background(), bad)
		require.ErrorIs(t, err, ErrInvalidStatusTransition)
		require.Equal(t, m.UploadStateFailed, repo.assets[asset.UUID].UploadState)
	}
	past := time.Now().Add(-time.Minute)
	repo.assets[asset.UUID].UploadState, repo.assets[asset.UUID].UploadExpiresAt = m.UploadStatePending, &past
	retried, err := svc.CreateAsset(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, m.UploadStatePending, retried.UploadState)
	require.True(t, repo.assets[asset.UUID].UploadExpiresAt.After(time.Now()))
}

func TestMediaExcelOriginalAndVariantCompleteOnLocalStorage(t *testing.T) {
	svc, repo := newUploadFixture(t)
	data, mime := officeFixture(t, "xlsx")
	checksum := payloadHash(data)
	ctx := context.Background()
	original, err := svc.CreateAsset(ctx, CreateAssetInput{TenantUUID: mediaTenantUUID, UploadMethod: UploadMethodPresign, Name: "book.xlsx", MimeType: mime, SizeBytes: int64(len(data)), ContentSHA256: checksum})
	require.NoError(t, err)
	ticket, err := svc.IssueHostTransferTicket(ctx, mediaTenantUUID, original.UUID, "upload", time.Minute)
	require.NoError(t, err)
	snapshot := authorizeUpload(t, svc, ticket)
	require.NoError(t, svc.PutHostTransfer(ctx, snapshot, bytes.NewReader(data), int64(len(data)), mime))
	// 上传 ticket 单次消费，完成确认仍需要读取真实对象。
	require.ErrorIs(t, svc.PutHostTransfer(ctx, snapshot, bytes.NewReader(data), int64(len(data)), mime), ErrUploadTicketExpired)
	ready, err := svc.CompleteAssetUpload(ctx, CompleteAssetUploadInput{TenantUUID: mediaTenantUUID, UUID: original.UUID, Checksum: checksum})
	require.NoError(t, err)
	require.Equal(t, m.UploadStateReady, ready.UploadState)
	in := CreateAssetVariantInput{TenantUUID: mediaTenantUUID, AssetUUID: ready.UUID, Variant: "preview", Name: "preview.xlsx", MimeType: mime, SizeBytes: int64(len(data)), Checksum: checksum}
	variant, err := svc.CreateAssetVariant(ctx, in)
	require.NoError(t, err)
	vt, err := svc.IssueVariantTransferTicket(ctx, mediaTenantUUID, ready.UUID, variant.UUID, "upload", time.Minute)
	require.NoError(t, err)
	_, err = svc.ConsumeVariantTransfer(ctx, ready.UUID, variant.UUID, "upload", strconv.FormatInt(vt.ExpiresAt.Unix(), 10), strconv.FormatUint(vt.Version, 10), strconv.FormatUint(vt.ParentVersion, 10), vt.Token, bytes.NewReader(data), int64(len(data)), mime)
	require.NoError(t, err)
	// 模拟旧 MIME 校验后的失败记录，磁盘已有完整对象。
	row := repo.variants[ready.UUID+"|preview"]
	row.UploadState, row.UploadExpiresAt = m.UploadStateFailed, nil
	row.TicketVersion++
	row.Meta = datatypes.JSON(`{"upload_failures":[{"reason":"mime_mismatch"}]}`)
	retried, err := svc.CreateAssetVariant(ctx, in)
	require.NoError(t, err)
	require.Equal(t, variant.UUID, retried.UUID)
	require.Equal(t, m.UploadStatePending, retried.UploadState)
	_, err = svc.ConsumeVariantTransfer(ctx, ready.UUID, variant.UUID, "upload", strconv.FormatInt(vt.ExpiresAt.Unix(), 10), strconv.FormatUint(vt.Version, 10), strconv.FormatUint(vt.ParentVersion, 10), vt.Token, bytes.NewReader(data), int64(len(data)), mime)
	require.ErrorIs(t, err, ErrUploadTicketExpired)
	newTicket, err := svc.IssueVariantTransferTicket(ctx, mediaTenantUUID, ready.UUID, variant.UUID, "upload", time.Minute)
	require.NoError(t, err)
	_, err = svc.ConsumeVariantTransfer(ctx, ready.UUID, variant.UUID, "upload", strconv.FormatInt(newTicket.ExpiresAt.Unix(), 10), strconv.FormatUint(newTicket.Version, 10), strconv.FormatUint(newTicket.ParentVersion, 10), newTicket.Token, bytes.NewReader(data), int64(len(data)), mime)
	require.NoError(t, err) // 重试能覆盖先前失败尝试留下的对象。
	completed, err := svc.CompleteVariantUpload(ctx, mediaTenantUUID, ready.UUID, variant.UUID, checksum)
	require.NoError(t, err)
	require.Equal(t, m.UploadStateReady, completed.UploadState)
	require.Contains(t, string(repo.variants[ready.UUID+"|preview"].Meta), "mime_mismatch")
}

func TestOfficeZIPDirectoryLimits(t *testing.T) {
	parts := make(map[string]string, maxOfficeEntries+1)
	for i := range maxOfficeEntries + 1 {
		parts[strconv.Itoa(i)+".txt"] = ""
	}
	data := zipFixture(t, parts)
	_, declared := officeFixture(t, "xlsx")
	v, err := validateUploadedObject(bytes.NewReader(data), int64(len(data)), int64(len(data)), declared, payloadHash(data), payloadHash(data))
	require.NoError(t, err)
	require.Equal(t, "invalid_office_package", v.Reason)
}
