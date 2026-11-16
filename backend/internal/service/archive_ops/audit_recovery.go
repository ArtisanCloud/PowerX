package archive_ops

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// 仅接收现有空消费维护器的已提交恢复批次，不解释任意 SQL、目录或用户上传的删除指令。
type auditRecoveryReceipt struct {
	State           string            `json:"state"`
	At              time.Time         `json:"at"`
	Cutoff          time.Time         `json:"cutoff"`
	Rows            int64             `json:"rows"`
	FirstID         int64             `json:"first_trace_id"`
	LastID          int64             `json:"last_trace_id"`
	RestoreVerified bool              `json:"restore_verified"`
	Schema          map[string]string `json:"schema_signatures"`
	Archives        []struct {
		File   string `json:"file"`
		Rows   int64  `json:"rows"`
		Bytes  int64  `json:"compressed_bytes"`
		SHA    string `json:"sha256"`
		RawSHA string `json:"binary_copy_sha256"`
	} `json:"archives"`
}
type recoveryFile struct {
	Path     string
	SHA      string
	Bytes    int64
	Modified time.Time
	Stamp    string
}
type recoveryPlan struct {
	Files       []recoveryFile
	Fingerprint string
	Bytes       int64
	Batches     int
	Protected   int
}

var recoveryRunName = regexp.MustCompile(`^apply-(?:[0-9]+|pilot)$`)
var recoveryBatchName = regexp.MustCompile(`^batch-[0-9]{8}$`)
var errRecoveryInvalid = errors.New("审计恢复归档不完整、已变更或无法核验，原始副本保持受保护")

// recoveryOpen 固定根目录后逐级 openat；中间目录、叶子软链及硬链接都不能改变读入对象。
func recoveryOpen(source Source, name string) (*os.File, error) {
	if source.AuditRecoveryDatabase == "" || source.FileOwnerUID == nil || filepath.Base(source.Directory) != source.AuditRecoveryDatabase || filepath.IsAbs(name) || strings.Contains(name, "\\") || filepath.Clean(name) != name {
		return nil, errRecoveryInvalid
	}
	fd, e := unix.Open(source.Directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, errRecoveryInvalid
	}
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Uid != *source.FileOwnerUID || st.Mode&0022 != 0 {
		unix.Close(fd)
		return nil, errRecoveryInvalid
	}
	parts := strings.Split(filepath.ToSlash(name), "/")
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			unix.Close(fd)
			return nil, errRecoveryInvalid
		}
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
		if i < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		next, e := unix.Openat(fd, part, flags, 0)
		unix.Close(fd)
		if e != nil {
			return nil, errRecoveryInvalid
		}
		fd = next
		if unix.Fstat(fd, &st) != nil || st.Uid != *source.FileOwnerUID || st.Mode&0022 != 0 {
			unix.Close(fd)
			return nil, errRecoveryInvalid
		}
		if i == len(parts)-1 && (st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Mode&0077 != 0) {
			unix.Close(fd)
			return nil, errRecoveryInvalid
		}
	}
	return os.NewFile(uintptr(fd), name), nil
}
func recoveryJSON(source Source, name string, out any) (recoveryFile, error) {
	f, e := recoveryOpen(source, name)
	if e != nil {
		return recoveryFile{}, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || st.Size() > 64<<10 {
		return recoveryFile{}, errRecoveryInvalid
	}
	raw, e := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if e != nil || int64(len(raw)) != st.Size() {
		return recoveryFile{}, errRecoveryInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		return recoveryFile{}, errRecoveryInvalid
	}
	h := sha256.Sum256(raw)
	return recoveryFile{Path: name, SHA: hex.EncodeToString(h[:]), Bytes: st.Size(), Modified: st.ModTime()}, nil
}
func recoveryArchive(ctx context.Context, source Source, name, sha, rawSHA string, size, expectedRows int64, verify bool) (recoveryFile, error) {
	if !shaPattern.MatchString(sha) || !shaPattern.MatchString(rawSHA) || size <= 0 || size > 1<<30 {
		return recoveryFile{}, errRecoveryInvalid
	}
	f, e := recoveryOpen(source, name)
	if e != nil {
		return recoveryFile{}, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || st.Size() != size {
		return recoveryFile{}, errRecoveryInvalid
	}
	var stat unix.Stat_t
	if unix.Fstat(int(f.Fd()), &stat) != nil {
		return recoveryFile{}, errRecoveryInvalid
	}
	out := recoveryFile{Path: name, SHA: sha, Bytes: size, Modified: st.ModTime(), Stamp: fmt.Sprintf("%d:%d:%d", stat.Dev, stat.Ino, stat.Ctim.Nano())}
	if !verify {
		return out, nil
	}
	h := sha256.New()
	buffer := bufio.NewReader(io.TeeReader(&contextReader{ctx, f}, h))
	g, e := gzip.NewReader(buffer)
	if e != nil {
		return recoveryFile{}, errRecoveryInvalid
	}
	g.Multistream(false)
	rawHash := sha256.New()
	limited := &io.LimitedReader{R: &contextReader{ctx, g}, N: (1 << 30) + 1}
	e = verifyRecoveryCOPY(io.TeeReader(limited, rawHash), expectedRows)
	closeErr := g.Close()
	if e != nil || closeErr != nil || limited.N <= 1 || hex.EncodeToString(rawHash.Sum(nil)) != rawSHA {
		return recoveryFile{}, errRecoveryInvalid
	}
	// 拒绝拼接第二份 gzip 或尾随数据；压缩文件与原始 COPY 的摘要同时匹配。
	if _, e = buffer.ReadByte(); e != io.EOF || hex.EncodeToString(h.Sum(nil)) != sha {
		return recoveryFile{}, errRecoveryInvalid
	}
	return out, nil
}

// verifyRecoveryCOPY 只解析 PostgreSQL binary COPY 的封装及行数，不执行 SQL 或恢复业务库。
func verifyRecoveryCOPY(r io.Reader, rows int64) error {
	head := make([]byte, 19)
	if _, e := io.ReadFull(r, head); e != nil || !bytes.Equal(head[:11], []byte("PGCOPY\n\xff\r\n\x00")) || binary.BigEndian.Uint32(head[11:15]) != 0 {
		return errRecoveryInvalid
	}
	ext := int64(binary.BigEndian.Uint32(head[15:19]))
	if ext > 64<<10 {
		return errRecoveryInvalid
	}
	if _, e := io.CopyN(io.Discard, r, ext); e != nil {
		return e
	}
	var count int64
	for {
		var width uint16
		if e := binary.Read(r, binary.BigEndian, &width); e != nil {
			return e
		}
		if width == 0xffff {
			var tail [1]byte
			n, e := r.Read(tail[:])
			if count != rows || n != 0 || e != io.EOF {
				return errRecoveryInvalid
			}
			return nil
		}
		if width == 0 || width > 1024 || count >= rows {
			return errRecoveryInvalid
		}
		for i := uint16(0); i < width; i++ {
			var size int32
			if e := binary.Read(r, binary.BigEndian, &size); e != nil {
				return e
			}
			if size == -1 {
				continue
			}
			if size < 0 || size > 64<<20 {
				return errRecoveryInvalid
			}
			if _, e := io.CopyN(io.Discard, r, int64(size)); e != nil {
				return e
			}
		}
		count++
	}
}

func auditRecoveryPlan(ctx context.Context, source Source, fullVerification ...bool) (recoveryPlan, error) {
	out := recoveryPlan{Files: []recoveryFile{}}
	if source.Kind != "database_files" || source.AuditRecoveryDatabase == "" {
		return out, errRecoveryInvalid
	}
	runs, e := os.ReadDir(source.Directory)
	if e != nil || len(runs) > 1000 {
		return out, errRecoveryInvalid
	}
	for _, run := range runs {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if !recoveryRunName.MatchString(run.Name()) {
			continue
		}
		if !run.IsDir() || run.Type()&os.ModeSymlink != 0 {
			return out, errRecoveryInvalid
		}
		// 原执行器的受限合同只作为历史证据保存；不从它读取或运行任何指令。
		contractPath := run.Name() + "/run-contract.json"
		contract, e := recoveryOpen(source, contractPath)
		if e != nil {
			return out, e
		}
		st, e := contract.Stat()
		if e != nil || st.Size() > 64<<10 {
			contract.Close()
			return out, errRecoveryInvalid
		}
		raw, e := io.ReadAll(io.LimitReader(contract, 64<<10+1))
		contract.Close()
		if e != nil || int64(len(raw)) != st.Size() {
			return out, errRecoveryInvalid
		}
		var proof struct {
			Automatic bool      `json:"automatic_backup_deletion"`
			Review    time.Time `json:"backup_review_after"`
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &proof) != nil || json.Unmarshal(raw, &fields) != nil || fields["automatic_backup_deletion"] == nil || proof.Automatic || proof.Review.IsZero() {
			return out, errRecoveryInvalid
		}
		h := sha256.Sum256(raw)
		contractFile := recoveryFile{Path: contractPath, SHA: hex.EncodeToString(h[:]), Bytes: st.Size(), Modified: st.ModTime()}
		batches, e := os.ReadDir(filepath.Join(source.Directory, run.Name()))
		if e != nil || len(batches) > 10000 {
			return out, errRecoveryInvalid
		}
		used := false
		for _, batch := range batches {
			if !recoveryBatchName.MatchString(batch.Name()) {
				continue
			}
			if !batch.IsDir() || batch.Type()&os.ModeSymlink != 0 {
				return out, errRecoveryInvalid
			}
			name := run.Name() + "/" + batch.Name() + "/receipt.json"
			var receipt auditRecoveryReceipt
			file, e := recoveryJSON(source, name, &receipt)
			if e != nil {
				return out, e
			}
			if receipt.State != "committed" {
				out.Protected++
				continue
			}
			if !receipt.RestoreVerified || receipt.At.IsZero() || receipt.At.After(time.Now().Add(time.Minute)) || receipt.Cutoff.IsZero() || !receipt.Cutoff.Before(receipt.At) || receipt.Rows < 1 || receipt.Rows > 5000 || receipt.FirstID < 1 || receipt.LastID < receipt.FirstID || receipt.Rows > receipt.LastID-receipt.FirstID+1 || len(receipt.Schema) != 2 || len(receipt.Archives) != 2 || receipt.Schema["public.capability_registry_invocation_traces"] == "" || receipt.Schema["public.capability_registry_event_publications"] == "" {
				return out, errRecoveryInvalid
			}
			for _, sig := range receipt.Schema {
				if sig == "" {
					return out, errRecoveryInvalid
				}
			}
			files := []recoveryFile{file}
			seen := map[string]bool{}
			for _, a := range receipt.Archives {
				if a.File != "traces.copy.gz" && a.File != "events.copy.gz" || seen[a.File] || a.Rows != receipt.Rows {
					return out, errRecoveryInvalid
				}
				seen[a.File] = true
				verify := len(fullVerification) == 0 || fullVerification[0]
				f, e := recoveryArchive(ctx, source, run.Name()+"/"+batch.Name()+"/"+a.File, a.SHA, a.RawSHA, a.Bytes, a.Rows, verify)
				if e != nil {
					return out, e
				}
				files = append(files, f)
			}
			out.Batches++
			out.Files = append(out.Files, files...)
			used = true
			if len(out.Files) > 10000 {
				return out, errRecoveryInvalid
			}
		}
		if used {
			out.Files = append(out.Files, contractFile)
		}
	}
	sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].Path < out.Files[j].Path })
	for _, f := range out.Files {
		out.Bytes += f.Bytes
		if out.Bytes > 4<<30 {
			return out, errors.New("审计恢复归档超过 4 GiB 接入预算，需核对保留及存储目标")
		}
	}
	raw, _ := json.Marshal(out.Files)
	h := sha256.Sum256(raw)
	out.Fingerprint = hex.EncodeToString(h[:])
	return out, nil
}

func (s *Service) captureAuditRecovery(ctx context.Context, j *jobRecord, source Source, entries map[string]Entry) error {
	plan, e := auditRecoveryPlan(ctx, source)
	if e != nil {
		return e
	}
	if plan.Batches == 0 {
		return errors.New("没有已提交且恢复验证通过的审计批次；原始内容保持保护")
	}
	for _, candidate := range plan.Files {
		f, e := recoveryOpen(source, candidate.Path)
		if e != nil {
			return e
		}
		entry, added, e := s.captureReader(ctx, io.LimitReader(f, candidate.Bytes+1), "database_files/"+candidate.Path, candidate.Bytes, candidate.Modified)
		f.Close()
		if e != nil {
			return e
		}
		if entry.SHA256 != candidate.SHA {
			return errRecoveryInvalid
		}
		entries[entry.Path] = entry
		j.NewBytes += added
	}
	j.Protected = true
	j.ProvenanceVerified = true
	j.AuditRecoveryFingerprint = plan.Fingerprint
	j.AuditRecoveryBatches = plan.Batches
	j.AuditRecoveryProtectedBatches = plan.Protected
	j.Warnings = append(j.Warnings, "审计恢复批次已复制纳管并默认保护；原始来源未删除。核对恢复证据后可由 Root 解除副本保护，按已发布策略清退。")
	return nil
}
