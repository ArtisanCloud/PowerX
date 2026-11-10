// Package archivecredentials 保存独立于业务数据库和普通归档导出的加密凭据。
package archivecredentials

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

var (
	ErrUnavailable      = errors.New("archive credential store unavailable")
	ErrInvalid          = errors.New("archive credential input invalid")
	ErrConflict         = errors.New("archive credential revision conflict")
	ErrRequestConflict  = errors.New("archive credential request conflict")
	ErrNotFound         = errors.New("archive credential not found")
	ErrRevoked          = errors.New("archive credential revoked")
	ErrRotationRequired = errors.New("archive credential key rotation required")
)
var label = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

const ledgerName = "credentials.sealed.json"
const maxLedger = 4 << 20

// Config 仅包含部署引用，不含密钥；目录与 keyring 文件必须预先由部署方准备。
type Config struct {
	Directory   string `json:"directory" yaml:"directory"`
	KeyringFile string `json:"keyring_file" yaml:"keyring_file"`
	StoreID     string `json:"store_id" yaml:"store_id"`
}

// Keyring 是独立挂载的密钥文件合同；服务不会生成或替换主密钥。
type Keyring struct {
	Version     int               `json:"version"`
	StoreID     string            `json:"store_id"`
	ActiveKeyID string            `json:"active_key_id"`
	Keys        map[string]string `json:"keys"`
}

// Value 永不进入公开视图、审计正文或普通配置导出。
type Value struct {
	Revision     int64  `json:"revision" yaml:"revision"`
	AccessKey    string `json:"access_key" yaml:"access_key"`
	SecretKey    string `json:"secret_key" yaml:"secret_key"`
	SessionToken string `json:"session_token,omitempty" yaml:"session_token,omitempty"`
}

// View 是经过认证的只读元数据。
type View struct {
	RecoveryEpoch  string    `json:"recovery_epoch,omitempty"`
	Ref            string    `json:"ref"`
	Revision       int64     `json:"revision"`
	State          string    `json:"state"`
	UpdatedAt      time.Time `json:"updated_at"`
	Actor          uint64    `json:"actor"`
	OriginRevision int64     `json:"origin_revision"`
	OriginDigest   string    `json:"origin_digest,omitempty"`
}
type record struct {
	View
	Value Value `json:"value"`
}

// Receipt 固定原操作结果；响应不明时只能读取它。
type Receipt struct {
	RecoveryEpoch  string    `json:"recovery_epoch,omitempty"`
	RequestID      string    `json:"request_id"`
	Ref            string    `json:"ref,omitempty"`
	Action         string    `json:"action"`
	Revision       int64     `json:"revision"`
	StoreRevision  int64     `json:"store_revision"`
	Actor          uint64    `json:"actor"`
	Trace          string    `json:"trace,omitempty"`
	OriginRevision int64     `json:"origin_revision,omitempty"`
	OriginDigest   string    `json:"origin_digest,omitempty"`
	At             time.Time `json:"at"`
	Digest         string    `json:"-"`
}
type storedReceipt struct {
	Receipt
	RequestDigest string `json:"request_digest"`
}

// Input 只允许固定操作；调用方负责真实 Root 用户授权。
type Input struct {
	RecoveryEpoch    string `json:"recovery_epoch,omitempty"`
	RequestID        string `json:"request_id"`
	Ref              string `json:"ref"`
	Action           string `json:"action"`
	ExpectedRevision int64  `json:"expected_revision"`
	Actor            uint64 `json:"actor"`
	Value            Value  `json:"value"`
	OriginRevision   int64  `json:"origin_revision"`
	OriginDigest     string `json:"origin_digest"`
	Trace            string `json:"-"`
}
type ledger struct {
	Version  int                      `json:"version"`
	StoreID  string                   `json:"store_id"`
	Scope    string                   `json:"scope"`
	Revision int64                    `json:"revision"`
	HashKey  []byte                   `json:"hash_key"`
	Records  map[string]record        `json:"records"`
	Receipts map[string]storedReceipt `json:"receipts"`
}
type header struct {
	Version int    `json:"version"`
	StoreID string `json:"store_id"`
	Scope   string `json:"scope"`
	KeyID   string `json:"key_id"`
}
type envelope struct {
	Header     header `json:"header"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

// Status 不含密钥路径或凭据值，缺少证明时保持 unavailable。
type Status struct {
	Configured       bool   `json:"configured"`
	Ready            bool   `json:"ready"`
	StoreID          string `json:"store_id,omitempty"`
	Revision         int64  `json:"revision"`
	KeyID            string `json:"key_id,omitempty"`
	ActiveKeyID      string `json:"active_key_id,omitempty"`
	RotationRequired bool   `json:"rotation_required"`
	Problem          string `json:"problem,omitempty"`
	Credentials      []View `json:"credentials"`
}

// Store 不创建主密钥、不缓存解密凭据，每次使用重新核对挂载密钥及目录身份。
type Store struct {
	cfg    Config
	scope  string
	mu     sync.Mutex
	device uint64
	inode  uint64
}

func New(c Config, scope string) (*Store, error) {
	if !label.MatchString(c.StoreID) || !label.MatchString(scope) || !filepath.IsAbs(c.Directory) || !filepath.IsAbs(c.KeyringFile) {
		return nil, ErrInvalid
	}
	rel, e := filepath.Rel(filepath.Clean(c.Directory), filepath.Clean(c.KeyringFile))
	if e != nil || rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != "..") {
		return nil, ErrInvalid
	}
	return &Store{cfg: c, scope: scope}, nil
}
func strict(raw []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		return ErrUnavailable
	}
	return nil
}
func secureDirectory(path string) (int, error) {
	for p := path; p != "/"; p = filepath.Dir(p) {
		var st unix.Stat_t
		if unix.Lstat(p, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR {
			return -1, ErrUnavailable
		}
	}
	fd, e := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if e != nil {
		return -1, ErrUnavailable
	}
	return fd, nil
}
func readAt(fd int, name string, limit int64) ([]byte, error) {
	f, e := unix.Openat(fd, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if e != nil {
		if errors.Is(e, unix.ENOENT) {
			return nil, ErrNotFound
		}
		return nil, ErrUnavailable
	}
	file := os.NewFile(uintptr(f), name)
	defer file.Close()
	var st unix.Stat_t
	if unix.Fstat(f, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || int(st.Uid) != os.Geteuid() || st.Mode&0077 != 0 || st.Size > limit {
		return nil, ErrUnavailable
	}
	raw, e := io.ReadAll(io.LimitReader(file, limit+1))
	if e != nil || int64(len(raw)) > limit {
		return nil, ErrUnavailable
	}
	return raw, nil
}
func (s *Store) keyring() (Keyring, error) {
	var k Keyring
	fd, e := secureDirectory(filepath.Dir(s.cfg.KeyringFile))
	if e != nil {
		return k, e
	}
	defer unix.Close(fd)
	raw, e := readAt(fd, filepath.Base(s.cfg.KeyringFile), 16<<10)
	if e != nil || strict(raw, &k) != nil || k.Version != 1 || k.StoreID != s.cfg.StoreID || !label.MatchString(k.ActiveKeyID) || len(k.Keys) < 1 || len(k.Keys) > 16 {
		return k, ErrUnavailable
	}
	if _, ok := k.Keys[k.ActiveKeyID]; !ok {
		return k, ErrUnavailable
	}
	for id, v := range k.Keys {
		b, e := base64.StdEncoding.DecodeString(v)
		if !label.MatchString(id) || e != nil || len(b) != 32 {
			return k, ErrUnavailable
		}
	}
	return k, nil
}
func gcm(k Keyring, id string) (cipher.AEAD, error) {
	b, e := base64.StdEncoding.DecodeString(k.Keys[id])
	if e != nil || len(b) != 32 {
		return nil, ErrUnavailable
	}
	block, e := aes.NewCipher(b)
	clear(b)
	if e != nil {
		return nil, ErrUnavailable
	}
	return cipher.NewGCM(block)
}
func (s *Store) withLedger(ctx context.Context, fn func(int, Keyring, *ledger, string) error) error {
	fd, e := secureDirectory(s.cfg.Directory)
	if e != nil {
		return e
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || int(st.Uid) != os.Geteuid() || st.Mode&0077 != 0 {
		return ErrUnavailable
	}
	s.mu.Lock()
	if s.inode == 0 {
		s.inode = st.Ino
		s.device = uint64(st.Dev)
	}
	same := s.inode == st.Ino && s.device == uint64(st.Dev)
	s.mu.Unlock()
	if !same {
		return ErrUnavailable
	}
	lock, e := unix.Openat(fd, "credentials.lock", unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return ErrUnavailable
	}
	defer unix.Close(lock)
	if unix.Fstat(lock, &st) != nil || st.Nlink != 1 || st.Mode&unix.S_IFMT != unix.S_IFREG || int(st.Uid) != os.Geteuid() || st.Mode&0077 != 0 {
		return ErrUnavailable
	}
	wait, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	for {
		if wait.Err() != nil {
			return ErrUnavailable
		}
		e = unix.Flock(lock, unix.LOCK_EX|unix.LOCK_NB)
		if e == nil {
			break
		}
		if !errors.Is(e, unix.EWOULDBLOCK) {
			return ErrUnavailable
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-wait.Done():
			timer.Stop()
			return ErrUnavailable
		case <-timer.C:
		}
	}
	defer unix.Flock(lock, unix.LOCK_UN)
	k, e := s.keyring()
	if e != nil {
		return e
	}
	raw, e := readAt(fd, ledgerName, maxLedger)
	d := ledger{Version: 1, StoreID: s.cfg.StoreID, Scope: s.scope, Records: map[string]record{}, Receipts: map[string]storedReceipt{}}
	if errors.Is(e, ErrNotFound) {
		return fn(fd, k, &d, "")
	}
	if e != nil {
		return e
	}
	var sealed envelope
	if strict(raw, &sealed) != nil || sealed.Header.Version != 1 || sealed.Header.StoreID != s.cfg.StoreID || sealed.Header.Scope != s.scope {
		return ErrUnavailable
	}
	a, e := gcm(k, sealed.Header.KeyID)
	if e != nil || len(sealed.Nonce) != a.NonceSize() {
		return ErrUnavailable
	}
	aad, _ := json.Marshal(sealed.Header)
	plain, e := a.Open(nil, sealed.Nonce, sealed.Ciphertext, aad)
	if e != nil {
		return ErrUnavailable
	}
	defer clear(plain)
	if strict(plain, &d) != nil || d.Version != 1 || d.StoreID != s.cfg.StoreID || d.Scope != s.scope || d.Revision < 1 || len(d.HashKey) != 32 || d.Records == nil || d.Receipts == nil || len(d.Records) > 256 || len(d.Receipts) > 2048 {
		return ErrUnavailable
	}
	return fn(fd, k, &d, sealed.Header.KeyID)
}
func (s *Store) persist(fd int, k Keyring, d *ledger) error {
	a, e := gcm(k, k.ActiveKeyID)
	if e != nil {
		return e
	}
	h := header{1, s.cfg.StoreID, s.scope, k.ActiveKeyID}
	aad, _ := json.Marshal(h)
	raw, e := json.Marshal(d)
	if e != nil {
		return ErrUnavailable
	}
	defer clear(raw)
	nonce := make([]byte, a.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return ErrUnavailable
	}
	sealed, e := json.Marshal(envelope{h, nonce, a.Seal(nil, nonce, raw, aad)})
	if e != nil || len(sealed) > maxLedger {
		return ErrUnavailable
	}
	name := ".sealed-" + uuid.NewString()
	f, e := unix.Openat(fd, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return ErrUnavailable
	}
	defer unix.Unlinkat(fd, name, 0)
	file := os.NewFile(uintptr(f), name)
	_, e = file.Write(sealed)
	if e == nil {
		e = file.Sync()
	}
	ce := file.Close()
	if e != nil || ce != nil {
		return ErrUnavailable
	}
	if unix.Renameat(fd, name, fd, ledgerName) != nil || unix.Fsync(fd) != nil {
		return ErrUnavailable
	}
	return nil
}

// Snapshot 只返回已认证的状态；锁或密钥不可用时不返回历史缓存。
func (s *Store) Snapshot(ctx context.Context) Status {
	out := Status{Configured: true, StoreID: s.cfg.StoreID, Credentials: []View{}}
	e := s.withLedger(ctx, func(_ int, k Keyring, d *ledger, key string) error {
		out.Ready = true
		out.Revision = d.Revision
		out.KeyID = key
		out.ActiveKeyID = k.ActiveKeyID
		out.RotationRequired = key != "" && key != k.ActiveKeyID
		for _, r := range d.Records {
			out.Credentials = append(out.Credentials, r.View)
		}
		return nil
	})
	if e != nil {
		out.Ready = false
		out.Problem = "ARCHIVE_CREDENTIAL_STORE_UNAVAILABLE"
	}
	return out
}

// Get 不允许已吊销凭据、丢失密钥或认证失败时回退到旧明文。
func (s *Store) Get(ctx context.Context, ref string) (Value, View, string, error) {
	var value Value
	var view View
	var identity string
	e := s.withLedger(ctx, func(_ int, k Keyring, d *ledger, key string) error {
		r, ok := d.Records[ref]
		if !ok {
			return ErrNotFound
		}
		view = r.View
		if r.State != "active" {
			return ErrRevoked
		}
		value = r.Value
		identity = s.cfg.StoreID + ":" + key + ":" + k.ActiveKeyID + ":" + ref + ":" + strconv.FormatInt(r.Revision, 10)
		return nil
	})
	return value, view, identity, e
}
func validValue(v Value) bool {
	return len(v.AccessKey) > 0 && len(v.AccessKey) <= 256 && len(v.SecretKey) > 0 && len(v.SecretKey) <= 1024 && len(v.SessionToken) <= 16384
}

// Validate 在记录接管绑定前检查固定 DTO，不写入任何文件。
func Validate(in Input) error {
	if in.RecoveryEpoch != "" {
		if id, e := uuid.Parse(in.RecoveryEpoch); e != nil || id.String() != in.RecoveryEpoch {
			return ErrInvalid
		}
	}
	if _, e := uuid.Parse(in.RequestID); e != nil || len(in.RequestID) != 36 || in.Actor == 0 || in.ExpectedRevision < 0 {
		return ErrInvalid
	}
	if in.Action != "save" && in.Action != "adopt" && in.Action != "revoke" && in.Action != "rewrap" || in.Action != "rewrap" && !label.MatchString(in.Ref) || (in.Action == "save" || in.Action == "adopt") && !validValue(in.Value) {
		return ErrInvalid
	}
	if in.Action == "adopt" && (len(in.OriginDigest) != 64 || in.OriginRevision < 0) {
		return ErrInvalid
	}

	return nil
}

// Mutate 将版本、审计回执与密文原子提交；任何已进入的持久化都不自动重试。
func (s *Store) Mutate(ctx context.Context, in Input) (Receipt, error) {
	var out Receipt
	if e := Validate(in); e != nil {
		return out, e
	}
	e := s.withLedger(ctx, func(fd int, k Keyring, d *ledger, key string) error {
		if len(d.HashKey) == 0 {
			d.HashKey = make([]byte, 32)
			if _, e := rand.Read(d.HashKey); e != nil {
				return ErrUnavailable
			}
		}
		body, _ := json.Marshal(in)
		hash := hmac.New(sha256.New, d.HashKey)
		hash.Write(body)
		digest := hex.EncodeToString(hash.Sum(nil))
		if old, ok := d.Receipts[in.RequestID]; ok {
			if old.RequestDigest != digest {
				return ErrRequestConflict
			}
			out = old.Receipt
			return nil
		}
		if len(d.Receipts) >= 2048 {
			return ErrUnavailable
		}
		if key != "" && key != k.ActiveKeyID && in.Action != "rewrap" {
			return ErrRotationRequired
		}
		if in.Action == "rewrap" {
			if d.Revision != in.ExpectedRevision || key == "" {
				return ErrConflict
			}
		} else {
			r, exists := d.Records[in.Ref]
			if r.Revision != in.ExpectedRevision {
				return ErrConflict
			}
			if in.Action == "adopt" && exists {
				return ErrConflict
			}
			if in.Action == "revoke" && !exists {
				return ErrNotFound
			}
			if !exists && len(d.Records) >= 256 {
				return ErrUnavailable
			}
			r.View.Ref = in.Ref
			r.RecoveryEpoch = in.RecoveryEpoch
			r.Revision++
			r.Actor = in.Actor
			r.UpdatedAt = time.Now().UTC()
			r.State = "active"
			if in.Action == "revoke" {
				r.State = "revoked"
				r.Value = Value{}
			} else {
				r.Value = in.Value
				r.Value.Revision = r.Revision
			}
			if in.Action == "adopt" {
				r.OriginRevision = in.OriginRevision
				r.OriginDigest = in.OriginDigest
			}
			d.Records[in.Ref] = r
			out.Revision = r.Revision
		}
		d.Revision++
		out.RequestID = in.RequestID
		out.RecoveryEpoch = in.RecoveryEpoch
		out.Action = in.Action
		out.Ref = in.Ref
		out.Actor = in.Actor
		out.Trace = in.Trace
		out.OriginRevision = in.OriginRevision
		out.OriginDigest = in.OriginDigest
		out.StoreRevision = d.Revision
		out.At = time.Now().UTC()
		d.Receipts[in.RequestID] = storedReceipt{out, digest}
		return s.persist(fd, k, d)
	})
	return out, e
}

// Request 仅允许原操作者只读核对，不回传摘要或凭据值。
func (s *Store) Request(ctx context.Context, id string, actor uint64) (Receipt, error) {
	var out Receipt
	if _, e := uuid.Parse(id); e != nil || len(id) != 36 {
		return out, ErrInvalid
	}
	e := s.withLedger(ctx, func(_ int, _ Keyring, d *ledger, _ string) error {
		r, ok := d.Receipts[id]
		if !ok {
			return ErrNotFound
		}
		if r.Actor != actor {
			return ErrRequestConflict
		}
		out = r.Receipt
		return nil
	})
	return out, e
}

// Paths 供既有 SSE 监听失效事件；不得将文件内容推送到浏览器。
func (s *Store) Paths() []string { return []string{s.cfg.Directory, filepath.Dir(s.cfg.KeyringFile)} }

// ReadPrivateFile 读取有界旧来源／绑定证据，拒绝软链、硬链和非私有属主。
func ReadPrivateFile(path string, limit int64) ([]byte, error) {
	fd, e := secureDirectory(filepath.Dir(path))
	if e != nil {
		return nil, e
	}
	defer unix.Close(fd)
	return readAt(fd, filepath.Base(path), limit)
}
