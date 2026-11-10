package archive_ops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	storagerecovery "github.com/ArtisanCloud/PowerX/internal/infra/storage_recovery"
	"os"
	"path/filepath"
	"sort"
	"strings"

	vault "github.com/ArtisanCloud/PowerX/internal/infra/archive_credentials"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
)

// CredentialOverview 只含可核对的引用与版本，不含凭据值。
type CredentialOverview struct {
	Store vault.Status     `json:"store"`
	Items []CredentialView `json:"items"`
}

// CredentialView 区分旧只读来源、加密权威、已吊销及未知状态。
type CredentialView struct {
	Ref          string `json:"ref"`
	Revision     int64  `json:"revision"`
	Origin       string `json:"origin"`
	State        string `json:"state"`
	LegacyDigest string `json:"legacy_digest,omitempty"`
}

// CredentialMutation 将原请求回执与当前权威状态分别返回。
type CredentialMutation struct {
	Receipt  vault.Receipt      `json:"receipt"`
	Overview CredentialOverview `json:"overview"`
}

// CredentialControlInput 不允许任意密钥路径或配置正文。
type CredentialControlInput struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	LegacyDigest     string `json:"legacy_digest,omitempty"`
	Confirm          bool   `json:"confirm"`
}

func outside(root, path string) bool {
	rel, e := filepath.Rel(root, path)
	return e == nil && rel != "." && rel != ".." && strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
func (s *Service) initCredentialVault() {
	if s.cfg.CredentialVault == nil {
		return
	}
	c := *s.cfg.CredentialVault
	// 只解析部署方路径；不创建目录、密钥或自动导入旧凭据。
	dir, e := filepath.EvalSymlinks(c.Directory)
	if e != nil {
		s.vaultErr = e
		return
	}
	keyParent, e := filepath.EvalSymlinks(filepath.Dir(c.KeyringFile))
	if e != nil {
		s.vaultErr = e
		return
	}
	c.Directory = dir
	c.KeyringFile = filepath.Join(keyParent, filepath.Base(c.KeyringFile))
	if !outside(s.cfg.Directory, c.Directory) || !outside(s.cfg.Directory, c.KeyringFile) || !outside(c.Directory, c.KeyringFile) {
		s.vaultErr = vault.ErrInvalid
		return
	}
	s.cfg.CredentialVault = &c
	s.vault, s.vaultErr = vault.New(c, s.cfg.ExecutionNodeID)
}
func (s *Service) vaultLocationDigest() string {
	if s.cfg.CredentialVault == nil {
		return ""
	}
	sum := sha256.Sum256([]byte(s.cfg.CredentialVault.Directory + "\x00" + s.cfg.CredentialVault.KeyringFile))
	return hex.EncodeToString(sum[:])
}

type credentialVaultBinding struct {
	Version  int              `json:"version"`
	StoreID  string           `json:"store_id"`
	Scope    string           `json:"scope"`
	Location string           `json:"location"`
	Minimum  map[string]int64 `json:"minimum_revisions"`
}

func (s *Service) credentialBindingPath() string {
	return filepath.Join(s.cfg.Directory, "credential-vault-binding.json")
}
func (s *Service) readCredentialBinding() (credentialVaultBinding, error) {
	var b credentialVaultBinding
	raw, e := vault.ReadPrivateFile(s.credentialBindingPath(), 32<<10)
	if e != nil {
		return b, e
	}
	if json.Unmarshal(raw, &b) != nil || b.Version != 1 || s.cfg.CredentialVault == nil || b.StoreID != s.cfg.CredentialVault.StoreID || b.Scope != s.cfg.ExecutionNodeID || b.Location != s.vaultLocationDigest() || b.Minimum == nil || len(b.Minimum) > 256 {
		return b, vault.ErrUnavailable
	}
	return b, nil
}
func (s *Service) credentialProviderBound() error {
	b, e := s.readCredentialBinding()
	if errors.Is(e, vault.ErrNotFound) {
		return nil
	}
	if e != nil || s.vault == nil {
		return vault.ErrUnavailable
	}
	status := s.vault.Snapshot(s.ctx)
	if !status.Ready || status.Revision == 0 {
		return vault.ErrUnavailable
	}
	actual := map[string]int64{}
	for _, r := range status.Credentials {
		actual[r.Ref] = r.Revision
	}
	for ref, min := range b.Minimum {
		if min < 1 || actual[ref] < min {
			return vault.ErrUnavailable
		}
	}
	return nil
}

// commitCredential 先写独立保护性版本下限，再提交密文；未知提交停在待核对，不能回退旧值。
func (s *Service) commitCredential(ctx context.Context, in vault.Input) (vault.Receipt, error) {
	var empty vault.Receipt
	if s.vault == nil || s.vaultErr != nil {
		return empty, vault.ErrUnavailable
	}
	unlock, e := s.lock(ctx, "credential-vault-binding")
	if e != nil {
		return empty, e
	}
	defer unlock()
	status := s.vault.Snapshot(ctx)
	if !status.Ready {
		return empty, vault.ErrUnavailable
	}
	b, e := s.readCredentialBinding()
	if e == nil {
		if s.credentialProviderBound() != nil {
			return empty, vault.ErrUnavailable
		}
	} else if errors.Is(e, vault.ErrNotFound) {
		b = credentialVaultBinding{Version: 1, StoreID: s.cfg.CredentialVault.StoreID, Scope: s.cfg.ExecutionNodeID, Location: s.vaultLocationDigest(), Minimum: map[string]int64{}}
	} else {
		return empty, e
	}
	// 同一请求仍由密文账本核对 HMAC，不能因当前版本已推进而误判成第二次写入。
	if _, err := s.vault.Request(ctx, in.RequestID, in.Actor); err == nil {
		return s.vault.Mutate(ctx, in)
	} else if !errors.Is(err, vault.ErrNotFound) {
		return empty, err
	}
	if in.Action != "rewrap" {
		_, view, _, err := s.vault.Get(ctx, in.Ref)
		if err != nil && !errors.Is(err, vault.ErrNotFound) && !errors.Is(err, vault.ErrRevoked) {
			return empty, err
		}
		if view.Revision != in.ExpectedRevision {
			return empty, vault.ErrConflict
		}
		if in.Action == "adopt" && view.Revision != 0 {
			return empty, vault.ErrConflict
		}
		if in.Action == "revoke" && view.Revision == 0 {
			return empty, vault.ErrNotFound
		}
		if len(b.Minimum) >= 256 && b.Minimum[in.Ref] == 0 {
			return empty, vault.ErrUnavailable
		}
		b.Minimum[in.Ref] = in.ExpectedRevision + 1
	} else if status.Revision != in.ExpectedRevision || status.KeyID == "" {
		return empty, vault.ErrConflict
	}
	if status.RotationRequired && in.Action != "rewrap" {
		return empty, vault.ErrRotationRequired
	}
	if e = writeJSON(s.credentialBindingPath(), b); e != nil {
		return empty, e
	}
	directory, err := os.Open(s.cfg.Directory)
	if err != nil {
		return empty, err
	}
	err = directory.Sync()
	closeErr := directory.Close()
	if err != nil {
		return empty, err
	}
	if closeErr != nil {
		return empty, closeErr
	}
	return s.vault.Mutate(ctx, in)
}
func (s *Service) credentialRoot(ctx context.Context) error {
	if !auth.IsPlatformRootSession(ctx) {
		return failure(403, "ARCHIVE_CREDENTIAL_ROOT_REQUIRED", "需要平台 Root 用户会话")
	}
	return s.check(ctx)
}
func credentialError(e error) error {
	switch {
	case e == nil:
		return nil
	case errors.Is(e, vault.ErrInvalid):
		return failure(400, "ARCHIVE_CREDENTIAL_INVALID", "凭据、请求编号或确认信息无效")
	case errors.Is(e, ErrBusy):
		return failure(409, "ARCHIVE_CREDENTIAL_BUSY", "另有凭据操作正在提交，请刷新版本后核对")
	case errors.Is(e, vault.ErrConflict):
		return failure(409, "ARCHIVE_REVISION_CONFLICT", "凭据版本已变化，请保留草稿并重新核对")
	case errors.Is(e, vault.ErrRequestConflict):
		return failure(409, "ARCHIVE_CREDENTIAL_REQUEST_CONFLICT", "此请求编号已用于不同操作")
	case errors.Is(e, vault.ErrNotFound):
		return failure(404, "ARCHIVE_CREDENTIAL_NOT_FOUND", "尚未找到此凭据或请求回执，请勿重发未知操作")
	case errors.Is(e, vault.ErrRevoked):
		return failure(422, "ARCHIVE_CREDENTIAL_REVOKED", "归档凭据已吊销")
	case errors.Is(e, vault.ErrRotationRequired):
		return failure(409, "ARCHIVE_CREDENTIAL_ROTATION_REQUIRED", "请先完成独立主密钥轮换")
	default:
		return failure(503, "ARCHIVE_CREDENTIAL_STORE_UNAVAILABLE", "独立凭据存储或密钥不可用，拒绝回退旧明文")
	}
}
func (s *Service) legacyS3Credential(ref string) (S3Credential, error) {
	if !labelPattern.MatchString(ref) {
		return S3Credential{}, vault.ErrInvalid
	}
	if c, ok := s.cfg.S3Credentials[ref]; ok && c.AccessKey != "" && c.SecretKey != "" {
		return c, nil
	}
	path := filepath.Join(s.cfg.Directory, "secrets", ref+".json")
	st, e := os.Lstat(path)
	if errors.Is(e, os.ErrNotExist) {
		return S3Credential{}, vault.ErrNotFound
	}
	if e != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		return S3Credential{}, vault.ErrUnavailable
	}
	var c S3Credential
	raw, readErr := vault.ReadPrivateFile(path, 32<<10)
	if readErr != nil || json.Unmarshal(raw, &c) != nil || c.AccessKey == "" || c.SecretKey == "" {
		return c, vault.ErrUnavailable
	}
	return c, nil
}
func legacyCredentialDigest(c S3Credential) string {
	b, _ := json.Marshal(c)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func (s *Service) credentialOverview(ctx context.Context) CredentialOverview {
	out := CredentialOverview{Items: []CredentialView{}, Store: vault.Status{Configured: s.cfg.CredentialVault != nil, Credentials: []vault.View{}}}
	if s.vault != nil {
		out.Store = s.vault.Snapshot(ctx)
	} else {
		out.Store.Problem = "ARCHIVE_CREDENTIAL_STORE_UNAVAILABLE"
	}
	if s.credentialProviderBound() != nil {
		out.Store.Ready = false
		out.Store.Problem = "ARCHIVE_CREDENTIAL_PROVIDER_REQUIRED"
	}
	refs, _ := s.s3CredentialRefs()
	epoch, epochError := s.recoveryEpoch()
	if epochError != nil {
		out.Store.Ready = false
		out.Store.Problem = "ARCHIVE_RECOVERY_UNAVAILABLE"
	}
	seen := map[string]bool{}
	for _, v := range out.Store.Credentials {
		seen[v.Ref] = true
		state := v.State
		if epoch != "" && v.RecoveryEpoch != epoch && state == "active" {
			state = "recovery_required"
		}
		if !out.Store.Ready || epochError != nil {
			state = "unavailable"
		}
		out.Items = append(out.Items, CredentialView{Ref: v.Ref, Revision: v.Revision, Origin: "sealed", State: state})
	}
	for _, ref := range refs {
		if seen[ref] {
			continue
		}
		v, e := s.legacyS3Credential(ref)
		row := CredentialView{Ref: ref, Origin: "legacy", State: "unavailable"}
		if e == nil {
			row.Revision = v.Revision
			row.LegacyDigest = legacyCredentialDigest(v)
			row.State = "read_only"
			if epoch != "" {
				row.State = "recovery_required"
			}
		}
		if out.Store.Configured && !out.Store.Ready {
			row.State = "unavailable"
		}
		out.Items = append(out.Items, row)
	}
	sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].Ref < out.Items[j].Ref })
	return out
}

// CredentialOverview 供 Root 查看权威与版本，不读取到浏览器可见的凭据值。
func (s *Service) CredentialOverview(ctx context.Context) (CredentialOverview, error) {
	if e := s.credentialRoot(ctx); e != nil {
		return CredentialOverview{}, e
	}
	return s.credentialOverview(ctx), nil
}
func (s *Service) mutateCredential(ctx context.Context, in vault.Input) (CredentialMutation, error) {
	if e := s.credentialRoot(ctx); e != nil {
		return CredentialMutation{}, e
	}
	in.Actor = reqctx.GetClaims(ctx).UserID
	in.Trace = reqctx.GetTraceID(ctx)
	epoch, epochError := s.recoveryEpoch()
	if epochError != nil {
		return CredentialMutation{}, epochError
	}
	if epoch != "" && in.Action == "adopt" {
		return CredentialMutation{}, failure(409, "ARCHIVE_CREDENTIAL_RECOVERY_REQUIRED", "恢复后须重新提交凭据，不能接管历史明文值")
	}
	in.RecoveryEpoch = epoch
	if e := vault.Validate(in); e != nil {
		return CredentialMutation{}, credentialError(e)
	}
	var receipt vault.Receipt
	var e error
	if s.recoveryAnchor != nil {
		e = s.recoveryAnchor.WithEpoch(ctx, func(current storagerecovery.Epoch) error {
			if current.Epoch != epoch {
				return failure(409, "ARCHIVE_RECOVERY_EPOCH_CHANGED", "恢复代次已变化")
			}
			var err error
			receipt, err = s.commitCredential(ctx, in)
			return err
		})
	} else {
		receipt, e = s.commitCredential(ctx, in)
	}
	if e != nil {
		return CredentialMutation{}, credentialError(e)
	}
	return CredentialMutation{receipt, s.credentialOverview(ctx)}, nil
}

// AdoptS3Credential 复制已核对的旧值到独立加密权威，旧对象仅保留证据。
func (s *Service) AdoptS3Credential(ctx context.Context, ref string, in CredentialControlInput) (CredentialMutation, error) {
	if e := s.credentialRoot(ctx); e != nil {
		return CredentialMutation{}, e
	}
	if !in.Confirm {
		return CredentialMutation{}, credentialError(vault.ErrInvalid)
	}
	if s.vault != nil {
		prior, err := s.vault.Request(ctx, in.RequestID, reqctx.GetClaims(ctx).UserID)
		if err == nil {
			if prior.Action != "adopt" || prior.Ref != ref || prior.OriginRevision != in.ExpectedRevision || prior.OriginDigest != in.LegacyDigest {
				return CredentialMutation{}, credentialError(vault.ErrRequestConflict)
			}
			return CredentialMutation{prior, s.credentialOverview(ctx)}, nil
		}
		if !errors.Is(err, vault.ErrNotFound) {
			return CredentialMutation{}, credentialError(err)
		}
	}
	old, e := s.legacyS3Credential(ref)
	if e != nil {
		return CredentialMutation{}, credentialError(e)
	}
	if old.Revision != in.ExpectedRevision || legacyCredentialDigest(old) != in.LegacyDigest {
		return CredentialMutation{}, credentialError(vault.ErrConflict)
	}
	return s.mutateCredential(ctx, vault.Input{Action: "adopt", RequestID: in.RequestID, Ref: ref, Value: old, OriginRevision: old.Revision, OriginDigest: in.LegacyDigest})
}

// RevokeS3Credential 吊销当前版本并清除密文内的活动凭据值，不删除审计回执。
func (s *Service) RevokeS3Credential(ctx context.Context, ref string, in CredentialControlInput) (CredentialMutation, error) {
	if e := s.credentialRoot(ctx); e != nil {
		return CredentialMutation{}, e
	}
	if !in.Confirm || in.LegacyDigest != "" {
		return CredentialMutation{}, credentialError(vault.ErrInvalid)
	}
	return s.mutateCredential(ctx, vault.Input{Action: "revoke", RequestID: in.RequestID, Ref: ref, ExpectedRevision: in.ExpectedRevision})
}

// RotateCredentialKey 只重新加密到部署方当前活动密钥，不接收主密钥值。
func (s *Service) RotateCredentialKey(ctx context.Context, in CredentialControlInput) (CredentialMutation, error) {
	if e := s.credentialRoot(ctx); e != nil {
		return CredentialMutation{}, e
	}
	if !in.Confirm || in.LegacyDigest != "" {
		return CredentialMutation{}, credentialError(vault.ErrInvalid)
	}
	return s.mutateCredential(ctx, vault.Input{Action: "rewrap", RequestID: in.RequestID, ExpectedRevision: in.ExpectedRevision})
}

// CredentialRequest 响应不明时只核对原操作者回执。
func (s *Service) CredentialRequest(ctx context.Context, id string) (CredentialMutation, error) {
	if e := s.credentialRoot(ctx); e != nil {
		return CredentialMutation{}, e
	}
	if s.vault == nil || s.credentialProviderBound() != nil {
		return CredentialMutation{}, credentialError(vault.ErrUnavailable)
	}
	receipt, e := s.vault.Request(ctx, id, reqctx.GetClaims(ctx).UserID)
	if errors.Is(e, vault.ErrRequestConflict) {
		return CredentialMutation{}, failure(403, "ARCHIVE_CREDENTIAL_REQUEST_FORBIDDEN", "仅原提交者可核对此请求回执")
	}
	if e != nil {
		return CredentialMutation{}, credentialError(e)
	}
	return CredentialMutation{receipt, s.credentialOverview(ctx)}, nil
}
