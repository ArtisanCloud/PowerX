package media

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

const (
	maxOfficeArchiveSize = 64 << 20
	maxOfficeEntries     = 4096
	maxOfficeDirectory   = 4 << 20
	maxOfficeExpanded    = 512 << 20
	maxContentTypesXML   = 256 << 10
	maxOfficeMainXML     = 4 << 20
)

type officeFormat struct{ mime, part, contentType, root, namespace string }

var officeFormats = []officeFormat{
	{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "xl/workbook.xml", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml", "workbook", "http://schemas.openxmlformats.org/spreadsheetml/2006/main"},
	{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "word/document.xml", "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml", "document", "http://schemas.openxmlformats.org/wordprocessingml/2006/main"},
	{"application/vnd.openxmlformats-officedocument.presentationml.presentation", "ppt/presentation.xml", "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml", "presentation", "http://schemas.openxmlformats.org/presentationml/2006/main"},
}

type uploadValidation struct {
	Reason            string
	DeclaredMIME      string
	ContainerMIME     string
	DetectedMIME      string
	ExpectedSize      int64
	ReportedSize      int64
	ActualSize        int64
	ChecksumMatches   bool
	ExpectedHashMatch bool
}

func normalizeUploadMIME(value string) string {
	typeName, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	switch strings.ToLower(typeName) {
	case "text/x-markdown", "text/md":
		return "text/markdown"
	case "image/jpg":
		return "image/jpeg"
	case "application/x-zip-compressed":
		return "application/zip"
	default:
		return strings.ToLower(typeName)
	}
}

// 原文件与 Variant 均从真实字节判定类型；不信任 S3 Content-Type 或文件后缀。
// OOXML 仅读取有界的目录及关键 XML，不展开文件到磁盘。
func validateUploadedObject(body io.Reader, reportedSize, expectedSize int64, declared, expectedHash, submittedHash string) (uploadValidation, error) {
	v := uploadValidation{DeclaredMIME: normalizeUploadMIME(declared), ExpectedSize: expectedSize, ReportedSize: reportedSize}
	if body == nil || expectedSize < 0 || expectedSize == int64(^uint64(0)>>1) {
		v.Reason = "size_mismatch"
		return v, nil
	}
	reader := bufio.NewReader(body)
	header, err := reader.Peek(512)
	if err != nil && err != io.EOF {
		return v, err
	}
	v.ContainerMIME = normalizeUploadMIME(http.DetectContentType(header))
	v.DetectedMIME = v.ContainerMIME
	var office *officeFormat
	for i := range officeFormats {
		if officeFormats[i].mime == v.DeclaredMIME {
			office = &officeFormats[i]
		}
	}
	var spool *os.File
	if office != nil && v.ContainerMIME == "application/zip" {
		if expectedSize > maxOfficeArchiveSize {
			v.Reason = "office_package_limit"
			return v, nil
		}
		spool, err = os.CreateTemp("", "powerx-office-validation-*")
		if err != nil {
			return v, err
		}
		defer func() { _ = spool.Close(); _ = os.Remove(spool.Name()) }()
	}
	hash := sha256.New()
	var sink io.Writer = hash
	if spool != nil {
		sink = io.MultiWriter(hash, spool)
	}
	v.ActualSize, err = io.Copy(sink, io.LimitReader(reader, expectedSize+1))
	if err != nil {
		return v, err
	}
	actualHash := hex.EncodeToString(hash.Sum(nil))
	v.ChecksumMatches = strings.EqualFold(actualHash, submittedHash)
	v.ExpectedHashMatch = strings.EqualFold(actualHash, expectedHash)
	switch {
	case v.ActualSize != expectedSize || reportedSize != expectedSize:
		v.Reason = "size_mismatch"
	case !v.ChecksumMatches || !v.ExpectedHashMatch:
		v.Reason = "checksum_mismatch"
	case v.DeclaredMIME == "":
		v.Reason = "mime_mismatch"
	case office != nil:
		if spool == nil || validateOfficePackage(spool, v.ActualSize, *office) != nil {
			v.Reason = "invalid_office_package"
		} else {
			v.DetectedMIME = office.mime
		}
	case v.DeclaredMIME == "text/markdown" || v.DeclaredMIME == "text/csv":
		if v.ContainerMIME != "text/plain" {
			v.Reason = "mime_mismatch"
		}
	case v.DeclaredMIME != v.DetectedMIME:
		v.Reason = "mime_mismatch"
	}
	return v, nil
}

func validateOfficePackage(file io.ReaderAt, size int64, format officeFormat) error {
	// 在 zip.NewReader 分配中央目录条目前，先检查普通 ZIP 的 EOCD。
	// 有界 Office 校验不接受 ZIP64 或跨磁盘包。
	tailSize := min(size, int64(65535+22))
	tail := make([]byte, tailSize)
	if _, err := file.ReadAt(tail, size-tailSize); err != nil {
		return err
	}
	end := -1
	for i := len(tail) - 22; i >= 0; i-- {
		if bytes.Equal(tail[i:i+4], []byte{'P', 'K', 5, 6}) && i+22+int(binary.LittleEndian.Uint16(tail[i+20:i+22])) == len(tail) {
			end = i
			break
		}
	}
	if end < 0 {
		return fmt.Errorf("office ZIP directory missing")
	}
	eocd := tail[end:]
	entries := binary.LittleEndian.Uint16(eocd[10:12])
	dirSize, dirOffset := binary.LittleEndian.Uint32(eocd[12:16]), binary.LittleEndian.Uint32(eocd[16:20])
	if binary.LittleEndian.Uint16(eocd[4:6]) != 0 || binary.LittleEndian.Uint16(eocd[6:8]) != 0 || binary.LittleEndian.Uint16(eocd[8:10]) != entries || entries == 0 || entries > maxOfficeEntries || dirSize > maxOfficeDirectory || int64(dirOffset)+int64(dirSize) > size-tailSize+int64(end) {
		return fmt.Errorf("office ZIP limits exceeded")
	}
	archive, err := zip.NewReader(file, size)
	if err != nil {
		return err
	}
	if len(archive.File) != int(entries) {
		return fmt.Errorf("office ZIP entry count mismatch")
	}
	parts := make(map[string]*zip.File, len(archive.File))
	var expanded uint64
	for _, entry := range archive.File {
		name := entry.Name
		if len(name) > 1024 || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || path.Clean(name) != strings.TrimSuffix(name, "/") || strings.HasPrefix(name, "../") || entry.Mode()&os.ModeSymlink != 0 || parts[name] != nil {
			return fmt.Errorf("invalid office ZIP entry")
		}
		if entry.UncompressedSize64 > maxOfficeExpanded || expanded > maxOfficeExpanded-entry.UncompressedSize64 {
			return fmt.Errorf("office expanded size exceeded")
		}
		expanded += entry.UncompressedSize64
		parts[name] = entry
	}
	contentTypes, err := readOfficeXML(parts["[Content_Types].xml"], maxContentTypesXML)
	if err != nil {
		return err
	}
	var types struct {
		XMLName  xml.Name `xml:"http://schemas.openxmlformats.org/package/2006/content-types Types"`
		Override []struct {
			PartName    string `xml:"PartName,attr"`
			ContentType string `xml:"ContentType,attr"`
		} `xml:"Override"`
	}
	if err = xml.Unmarshal(contentTypes, &types); err != nil {
		return err
	}
	matched := false
	for _, entry := range types.Override {
		if entry.PartName == "/"+format.part {
			if matched || entry.ContentType != format.contentType {
				return fmt.Errorf("office main content type mismatch")
			}
			matched = true
		}
	}
	if !matched {
		return fmt.Errorf("office main content type missing")
	}
	main, err := readOfficeXML(parts[format.part], maxOfficeMainXML)
	if err != nil {
		return err
	}
	decoder := xml.NewDecoder(bytes.NewReader(main))
	rootSeen := false
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			if !rootSeen || depth != 0 {
				return fmt.Errorf("office main XML incomplete")
			}
			return nil
		}
		if err != nil {
			return err
		}
		switch token := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				if rootSeen || token.Name.Local != format.root || token.Name.Space != format.namespace {
					return fmt.Errorf("office main XML root mismatch")
				}
				rootSeen = true
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.Directive:
			return fmt.Errorf("office XML directive unsupported")
		case xml.CharData:
			if depth == 0 && len(bytes.TrimSpace(token)) != 0 {
				return fmt.Errorf("office XML content outside root")
			}
		}
	}
}

func readOfficeXML(part *zip.File, limit uint64) ([]byte, error) {
	if part == nil || part.UncompressedSize64 > limit {
		return nil, fmt.Errorf("office XML missing or exceeds limit")
	}
	reader, err := part.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	if err != nil || uint64(len(data)) > limit {
		return nil, fmt.Errorf("office XML read failed or exceeds limit")
	}
	return data, nil
}

func (v uploadValidation) fields() map[string]any {
	return map[string]any{"reason": v.Reason, "declared_mime": v.DeclaredMIME, "container_mime": v.ContainerMIME, "detected_mime": v.DetectedMIME, "expected_size": v.ExpectedSize, "reported_size": v.ReportedSize, "actual_size": v.ActualSize, "checksum_matches": v.ChecksumMatches, "expected_checksum_matches": v.ExpectedHashMatch}
}

func logUploadValidation(ctx context.Context, tenant, asset, variant string, v uploadValidation) {
	fields := v.fields()
	fields["tenant_uuid"], fields["asset_uuid"], fields["variant_uuid"], fields["stage"] = tenant, asset, variant, "complete_upload"
	logger.WarnF(logger.WithLogFields(ctx, fields), "[media-upload] validation failed")
}
