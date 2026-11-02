package archive_ops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

// S3Credential 只存于私有运维配置／secrets 目录，不进入概览或任务清单。
type S3Credential struct {
	Revision     int64  `json:"revision" yaml:"revision"`
	AccessKey    string `json:"access_key" yaml:"access_key"`
	SecretKey    string `json:"secret_key" yaml:"secret_key"`
	SessionToken string `json:"session_token,omitempty" yaml:"session_token,omitempty"`
}
type SaveS3CredentialInput struct {
	Ref              string `json:"ref"`
	ExpectedRevision int64  `json:"expected_revision"`
	AccessKey        string `json:"access_key"`
	SecretKey        string `json:"secret_key"`
	SessionToken     string `json:"session_token,omitempty"`
}
type S3CredentialView struct {
	Ref      string `json:"ref"`
	Revision int64  `json:"revision"`
}

func (s *Service) s3Credential(ref string) (S3Credential, error) {
	if !labelPattern.MatchString(ref) {
		return S3Credential{}, failure(400, "ARCHIVE_CREDENTIAL_INVALID", "凭据引用无效")
	}
	if c, ok := s.cfg.S3Credentials[ref]; ok && c.AccessKey != "" && c.SecretKey != "" {
		return c, nil
	}
	p := filepath.Join(s.cfg.Directory, "secrets", ref+".json")
	st, err := os.Lstat(p)
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		return S3Credential{}, failure(422, "ARCHIVE_CREDENTIAL_UNAVAILABLE", "独立归档凭据未配置或权限不正确")
	}
	var c S3Credential
	if err = readJSON(p, &c); err != nil || c.AccessKey == "" || c.SecretKey == "" {
		return c, failure(422, "ARCHIVE_CREDENTIAL_UNAVAILABLE", "独立归档凭据不可用")
	}
	return c, nil
}
func (s *Service) s3CredentialRefs() ([]string, error) {
	refs := map[string]bool{}
	for ref := range s.cfg.S3Credentials {
		refs[ref] = true
	}
	entries, err := os.ReadDir(filepath.Join(s.cfg.Directory, "secrets"))
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		ref := strings.TrimSuffix(e.Name(), ".json")
		if !e.IsDir() && e.Name() == ref+".json" && labelPattern.MatchString(ref) {
			refs[ref] = true
		}
	}
	out := []string{}
	for ref := range refs {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out, nil
}
func (s *Service) SaveS3Credential(ctx context.Context, in SaveS3CredentialInput) (S3CredentialView, error) {
	var out S3CredentialView
	if err := s.check(ctx); err != nil {
		return out, err
	}
	if !labelPattern.MatchString(in.Ref) || in.AccessKey == "" || in.SecretKey == "" || len(in.AccessKey) > 256 || len(in.SecretKey) > 1024 || len(in.SessionToken) > 16384 || in.ExpectedRevision < 0 {
		return out, failure(400, "ARCHIVE_CREDENTIAL_INVALID", "请填写独立归档凭据及版本")
	}
	if _, configured := s.cfg.S3Credentials[in.Ref]; configured {
		return out, failure(409, "ARCHIVE_CREDENTIAL_DEPLOYED", "此凭据由部署配置管理，请创建新的凭据引用")
	}
	unlock, err := s.lock(ctx, "settings")
	if err != nil {
		return out, ErrBusy
	}
	defer unlock()
	p := filepath.Join(s.cfg.Directory, "secrets", in.Ref+".json")
	var old S3Credential
	if _, err = os.Lstat(p); err == nil {
		if err = readJSON(p, &old); err != nil {
			return out, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return out, err
	}
	if old.Revision != in.ExpectedRevision {
		return out, failure(409, "ARCHIVE_REVISION_CONFLICT", "凭据已被修改，请使用新的引用或刷新版本")
	}
	c := S3Credential{Revision: old.Revision + 1, AccessKey: in.AccessKey, SecretKey: in.SecretKey, SessionToken: in.SessionToken}
	if err = s.auditAction(ctx, "credential_change_intent", in.Ref, c.Revision); err != nil {
		return out, err
	}
	if err = writeJSON(p, c); err != nil {
		return out, err
	}
	s.auditAction(ctx, "credential_saved", in.Ref, c.Revision)
	return S3CredentialView{Ref: in.Ref, Revision: c.Revision}, nil
}
func validS3Target(t Target) bool {
	u, err := url.Parse(t.Endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())) {
		return false
	}
	return regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`).MatchString(t.Bucket) && cleanEntryPath(t.Prefix) && len(t.Prefix) <= 256 && !strings.HasSuffix(t.Prefix, "/") && labelPattern.MatchString(t.CredentialRef)
}
func (s *Service) saveS3Target(ctx context.Context, t Target) (Target, error) {
	if t.Name == "" || len(t.Name) > 128 || !validS3Target(t) {
		return t, failure(400, "ARCHIVE_TARGET_INVALID", "请检查 HTTPS 地址、桶、专用前缀和独立凭据引用；本机 HTTP 仅用于明确配置的测试目标")
	}
	if _, err := s.s3Credential(t.CredentialRef); err != nil {
		return t, err
	}
	unlock, err := s.lock(ctx, "settings")
	if err != nil {
		return t, ErrBusy
	}
	defer unlock()
	if t.ID == "" {
		t.ID = uuid.NewString()
		if t.Revision != 0 {
			return t, failure(409, "ARCHIVE_REVISION_CONFLICT", "新目标版本必须为零")
		}
	} else {
		old, err := s.target(t.ID)
		if err != nil {
			return t, err
		}
		if old.Revision != t.Revision {
			return t, failure(409, "ARCHIVE_REVISION_CONFLICT", "目标已被修改，请刷新后重试")
		}
	}
	t.Revision++
	t.VerifiedAt = nil
	t.VerifiedFingerprint = ""
	t.Executor = "powerx:" + s.cfg.ExecutionNodeID
	t.Host = ""
	t.User = ""
	t.Port = 0
	t.Directory = ""
	if err = s.auditAction(ctx, "target_change_intent", t.ID, t.Revision); err != nil {
		return t, err
	}
	if err = writeJSON(s.path("targets", t.ID), t); err != nil {
		return t, err
	}
	s.auditAction(ctx, "target_saved", t.ID, t.Revision)
	return t, nil
}
func (s *Service) s3Client(t Target) (*minio.Client, error) {
	if !validS3Target(t) {
		return nil, failure(422, "ARCHIVE_TARGET_INVALID", "归档目标配置无效")
	}
	c, err := s.s3Credential(t.CredentialRef)
	if err != nil {
		return nil, err
	}
	return minio.New(t.Endpoint, &minio.Options{AccessKey: c.AccessKey, SecretKey: c.SecretKey, SessionToken: c.SessionToken, Region: t.Region, ForcePathStyle: t.ForcePathStyle, HTTPClient: &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}})
}
func (s *Service) targetFingerprint(t Target) (string, error) {
	t.VerificationValid = false
	t.VerifiedAt = nil
	t.VerifiedFingerprint = ""
	t.Executor = ""
	raw, _ := json.Marshal(t)
	node, _ := json.Marshal(struct {
		NodeID       string
		HostIdentity string
		Instance     string
	}{s.cfg.ExecutionNodeID, s.hostIdentity, s.cfg.Instance})
	raw = append(raw, node...)
	if t.Driver == "s3" {
		c, err := s.s3Credential(t.CredentialRef)
		if err != nil {
			return "", err
		}
		secret, _ := json.Marshal(c)
		raw = append(raw, secret...)
	} else {
		credential, ok := s.cfg.Credentials[t.CredentialRef]
		if !ok {
			return "", errors.New("归档 SSH 凭据引用不可用")
		}
		for _, path := range []string{credential.KeyFile, credential.KnownHostsFile} {
			data, err := os.ReadFile(path)
			if err != nil || len(data) > 1<<20 {
				return "", errors.New("归档 SSH 凭据不可读取")
			}
			sum := sha256.Sum256(data)
			raw = append(raw, sum[:]...)
		}
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:]), nil
}
func (s *Service) targetVerified(t Target) bool {
	if t.VerifiedAt == nil || time.Since(*t.VerifiedAt) > 24*time.Hour || t.VerifiedAt.After(time.Now().Add(time.Minute)) {
		return false
	}
	fp, err := s.targetFingerprint(t)
	return err == nil && fp == t.VerifiedFingerprint
}
func missingS3Config(err error, codes ...string) bool {
	code := minio.ToErrorResponse(err).Code
	for _, v := range codes {
		if code == v {
			return true
		}
	}
	return false
}
func checkPrivateVersionedBucket(ctx context.Context, c *minio.Client, t Target) error {
	enabled, err := c.BucketVersioningEnabled(ctx, t.Bucket)
	if err != nil || !enabled {
		return errors.New("归档桶必须已开启版本保留，且执行节点有查询权限")
	}
	raw, err := c.BucketPolicy(ctx, t.Bucket)
	if err != nil && !missingS3Config(err, "NoSuchBucketPolicy", "NoSuchPolicy") {
		return errors.New("无法核对归档桶公开策略")
	}
	if err == nil {
		var p struct {
			Statement []struct {
				Effect       string
				Principal    any
				NotPrincipal any
			}
		}
		if json.Unmarshal(raw, &p) != nil {
			return errors.New("归档桶策略不可验证")
		}
		for _, st := range p.Statement {
			if st.Effect == "Allow" && (st.Principal == nil || st.NotPrincipal != nil || publicPrincipal(st.Principal)) {
				return errors.New("归档桶不得允许公开访问")
			}
		}
	}
	raw, err = c.BucketACL(ctx, t.Bucket)
	if err != nil {
		return errors.New("无法核对归档桶访问权限")
	}
	var acl struct {
		Grants []struct {
			URI string `xml:"Grantee>URI"`
		} `xml:"AccessControlList>Grant"`
	}
	if xml.Unmarshal(raw, &acl) != nil {
		return errors.New("归档桶权限不可验证")
	}
	for _, g := range acl.Grants {
		if strings.HasSuffix(g.URI, "/AllUsers") || strings.HasSuffix(g.URI, "/AuthenticatedUsers") {
			return errors.New("归档桶不得允许公开访问")
		}
	}
	raw, err = c.BucketLifecycle(ctx, t.Bucket)
	if err != nil && !missingS3Config(err, "NoSuchLifecycleConfiguration", "NoSuchLifecycle") {
		return errors.New("无法核对归档桶的自动过期规则")
	}
	if err == nil {
		var life struct {
			Rules []struct {
				Status                      string
				Expiration                  *struct{}
				NoncurrentVersionExpiration *struct{}
			} `xml:"Rule"`
		}
		if xml.Unmarshal(raw, &life) != nil {
			return errors.New("归档桶过期规则不可验证")
		}
		for _, r := range life.Rules {
			if r.Status == "Enabled" && (r.Expiration != nil || r.NoncurrentVersionExpiration != nil) {
				return errors.New("请移除绕过引用保护的桶级过期规则，使用 PowerX 归档保留策略")
			}
		}
	}
	return nil
}
func publicPrincipal(v any) bool {
	switch x := v.(type) {
	case string:
		return x == "*"
	case map[string]any:
		for _, v := range x {
			if publicPrincipal(v) {
				return true
			}
		}
	case []any:
		for _, v := range x {
			if publicPrincipal(v) {
				return true
			}
		}
	}
	return false
}
func readS3Version(ctx context.Context, c *minio.Client, t Target, key, version string, size int64, digest string, w io.Writer) error {
	if version == "" || version == "null" || size < 0 || !shaPattern.MatchString(digest) || !strings.HasPrefix(key, t.Prefix+"/") {
		return errors.New("归档对象版本或范围无效")
	}
	f, err := c.GetObject(ctx, t.Bucket, key, minio.GetObjectOptions{VersionID: version})
	if err != nil {
		return errors.New("归档对象不可读取")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.VersionID != version || st.Size != size {
		return errors.New("归档对象版本或长度不符")
	}
	h := sha256.New()
	if w == nil {
		w = io.Discard
	}
	n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(f, size+1))
	if err != nil || n != size || hex.EncodeToString(h.Sum(nil)) != digest {
		return errors.New("归档对象内容校验失败")
	}
	return nil
}
func (s *Service) testS3Target(ctx context.Context, t Target) error {
	c, err := s.s3Client(t)
	if err != nil {
		return err
	}
	if err = checkPrivateVersionedBucket(ctx, c, t); err != nil {
		return err
	}
	data := []byte("PowerX archive verification " + uuid.NewString())
	sum := sha256.Sum256(data)
	key := t.Prefix + "/_tests/" + uuid.NewString()
	info, err := c.PutObject(ctx, t.Bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil {
		return errors.New("执行节点无法写入归档测试对象")
	}
	if info.VersionID == "" || info.VersionID == "null" {
		return errors.New("测试对象没有独立版本，归档未启用")
	}
	defer func() {
		run, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = c.RemoveObject(run, t.Bucket, key, minio.RemoveObjectOptions{VersionID: info.VersionID})
	}()
	return readS3Version(ctx, c, t, key, info.VersionID, int64(len(data)), hex.EncodeToString(sum[:]), nil)
}
