package tenantkeys

import (
	"context"
	"encoding/base64" // 仅 DirectWrapper 构造时可能用到
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	repotenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"

	"github.com/ArtisanCloud/PowerX/pkg/crypto"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// --- 可注入的主密钥包装接口 ---
type KeyWrapper interface {
	WrapPrivateKey(privPEM []byte) (*crypto.Wrapped, error)
	UnwrapPrivateKey(w *crypto.Wrapped) ([]byte, error)
}

// 默认实现：保持兼容，仍走环境变量（老逻辑）
type envWrapper struct{}

func (envWrapper) WrapPrivateKey(privPEM []byte) (*crypto.Wrapped, error) {
	return crypto.WrapPrivateKey(privPEM)
}
func (envWrapper) UnwrapPrivateKey(w *crypto.Wrapped) ([]byte, error) {
	return crypto.UnwrapPrivateKey(w)
}

// 直接使用 config 注入的主密钥（不经由环境变量）
type directWrapper struct {
	id  string
	key []byte // 32 bytes
}

// 工具：用 base64 或原始 bytes 构造
func NewDirectWrapper(id string, keyB64 string, keyRaw []byte) (*directWrapper, error) {
	if id == "" {
		return nil, fmt.Errorf("wrap master key id 不能为空")
	}
	var k []byte
	switch {
	case len(keyRaw) == 32:
		k = keyRaw
	case keyB64 != "":
		dec, err := base64.StdEncoding.DecodeString(keyB64)
		if err != nil {
			return nil, fmt.Errorf("master key b64 非法: %w", err)
		}
		if len(dec) != 32 {
			return nil, fmt.Errorf("master key 长度必须是 32 字节，当前=%d", len(dec))
		}
		k = dec
	default:
		return nil, fmt.Errorf("需要提供 32 字节 keyRaw 或 base64 keyB64")
	}
	return &directWrapper{id: id, key: k}, nil
}

func (d *directWrapper) WrapPrivateKey(privPEM []byte) (*crypto.Wrapped, error) {
	return crypto.WrapPrivateKeyWith(d.id, d.key, privPEM)
}
func (d *directWrapper) UnwrapPrivateKey(w *crypto.Wrapped) ([]byte, error) {
	return crypto.UnwrapPrivateKeyWith(d.id, d.key, w)
}

// Service: 每租户一对 RSA 密钥对；公钥加密 API Key 入库；私钥用主密钥包裹后入库。
type TenantKeyService struct {
	db     *gorm.DB
	kpRepo *repotenant.TenantKeyPairRepository

	// 可注入的主密钥包装器（默认 envWrapper）
	wrapper KeyWrapper
}

// 兼容旧构造：默认用环境变量版
func NewTenantKeyService(db *gorm.DB) *TenantKeyService {
	return &TenantKeyService{
		db:      db,
		kpRepo:  repotenant.NewTenantKeyPairRepository(db),
		wrapper: envWrapper{},
	}
}

// 新构造：显式注入 wrapper（推荐在有 config 时使用）
func NewTenantKeyServiceWithWrapper(db *gorm.DB, w KeyWrapper) *TenantKeyService {
	if w == nil {
		w = envWrapper{}
	}
	return &TenantKeyService{db: db, kpRepo: repotenant.NewTenantKeyPairRepository(db), wrapper: w}
}

// EnsureActiveKeyPair：确保 (env, tenantID) 有激活密钥对；没有则生成一把。
func (s *TenantKeyService) EnsureActiveKeyPair(ctx context.Context, env string, tenantID *uint64) (*modeltenant.TenantKeyPair, error) {
	if kp, err := s.kpRepo.GetActiveByScope(ctx, env, tenantID); err == nil {
		return kp, nil
	}

	pubPEM, privPEM, err := crypto.GenerateRSA()
	if err != nil {
		return nil, err
	}

	// 使用注入的 wrapper（可来自 config），而不是固定读 env
	w, err := s.wrapper.WrapPrivateKey(privPEM)
	if err != nil {
		return nil, err
	}

	kid := "t:global"
	if tenantID != nil {
		kid = fmt.Sprintf("t:%d:v1", *tenantID)
	}

	kp := &modeltenant.TenantKeyPair{
		ScopeRef:   coremodel.ScopeRef{Env: env, TenantID: tenantID},
		KID:        kid,
		Alg:        "RSA-OAEP-256",
		PublicPEM:  string(pubPEM),
		EncPrivate: datatypes.JSONMap{"wrapAlg": w.WrapAlg, "nonce": w.Nonce, "ct": w.CT, "ts": w.TS},
		Active:     true,
	}

	return kp, s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := repotenant.NewTenantKeyPairRepository(tx)
		if err := txRepo.DeactivateAll(ctx, env, tenantID); err != nil {
			return err
		}
		return txRepo.Create(ctx, kp)
	})
}

// ActiveKeyPair 返回当前 scope 的激活密钥。
func (s *TenantKeyService) ActiveKeyPair(ctx context.Context, env string, tenantID *uint64) (*modeltenant.TenantKeyPair, error) {
	return s.kpRepo.GetActiveByScope(ctx, env, tenantID)
}

// RotateKeyPair 生成新密钥对并替换为激活状态。
func (s *TenantKeyService) RotateKeyPair(ctx context.Context, env string, tenantID *uint64) (*modeltenant.TenantKeyPair, error) {
	old, _ := s.kpRepo.GetActiveByScope(ctx, env, tenantID)

	pubPEM, privPEM, err := crypto.GenerateRSA()
	if err != nil {
		return nil, err
	}
	w, err := s.wrapper.WrapPrivateKey(privPEM)
	if err != nil {
		return nil, err
	}

	kp := &modeltenant.TenantKeyPair{
		ScopeRef: coremodel.ScopeRef{
			Env:      env,
			TenantID: tenantID,
		},
		KID:       nextKeyID(old, tenantID),
		Alg:       "RSA-OAEP-256",
		PublicPEM: string(pubPEM),
		EncPrivate: datatypes.JSONMap{
			"wrapAlg": w.WrapAlg,
			"nonce":   w.Nonce,
			"ct":      w.CT,
			"ts":      w.TS,
		},
		Active: true,
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := repotenant.NewTenantKeyPairRepository(tx)
		if err := repo.DeactivateAll(ctx, env, tenantID); err != nil {
			return err
		}
		return repo.Create(ctx, kp)
	})
	if err != nil {
		return nil, err
	}
	return kp, nil
}

// SealSensitive：把 data 中 keys… 的明文装进 data["__sealed"]（公钥加密），并删除这些明文键。
func (s *TenantKeyService) SealSensitive(ctx context.Context, env string, tenantID *uint64, data datatypes.JSONMap, keys ...string) (datatypes.JSONMap, error) {
	if data == nil {
		data = datatypes.JSONMap{}
	}
	kp, err := s.EnsureActiveKeyPair(ctx, env, tenantID)
	if err != nil {
		return nil, err
	}

	secret := map[string]any{}
	for _, k := range keys {
		if v, ok := data[k]; ok {
			secret[k] = v
			delete(data, k)
		}
	}
	if len(secret) == 0 {
		return data, nil
	}

	sealed, err := crypto.SealJSONWithPub([]byte(kp.PublicPEM), kp.KID, secret)
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(sealed)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	data["__sealed"] = m
	return data, nil
}

// UnsealSensitive：仅在后端需要明文时使用（例如连通性测试），解开 data["__sealed"] 到 out。
func (s *TenantKeyService) UnsealSensitive(ctx context.Context, env string, tenantID *uint64, data datatypes.JSONMap, out any) error {
	if data == nil {
		return errors.New("no data")
	}
	v, ok := data["__sealed"]
	if !ok || v == nil {
		return fmt.Errorf("no sealed")
	}
	b, _ := json.Marshal(v)
	var sealed crypto.Sealed
	if err := json.Unmarshal(b, &sealed); err != nil {
		return err
	}

	kp, err := s.kpRepo.GetActiveByScope(ctx, env, tenantID)
	if err != nil {
		return err
	}
	w, err := wrappedFromJSONMap(kp.EncPrivate)
	if err != nil {
		return err
	}

	// 使用注入的 wrapper 解包（不再读环境变量）
	privPEM, err := s.wrapper.UnwrapPrivateKey(&w)
	if err != nil {
		return err
	}
	return crypto.UnsealJSONWithPriv(privPEM, &sealed, out)
}

// --- helpers ---
func wrappedFromJSONMap(m datatypes.JSONMap) (crypto.Wrapped, error) {
	if m == nil {
		return crypto.Wrapped{}, errors.New("empty wrapped")
	}
	wa, _ := m["wrapAlg"].(string)
	nonce, _ := m["nonce"].(string)
	ct, _ := m["ct"].(string)
	if wa != "" && nonce != "" && ct != "" {
		return crypto.Wrapped{WrapAlg: wa, Nonce: nonce, CT: ct}, nil
	}
	b, _ := json.Marshal(m)
	var w crypto.Wrapped
	if err := json.Unmarshal(b, &w); err != nil {
		return crypto.Wrapped{}, fmt.Errorf("invalid wrapped json")
	}
	return w, nil
}

func nextKeyID(old *modeltenant.TenantKeyPair, tenantID *uint64) string {
	prefix := baseKeyPrefix(tenantID)
	version := 1
	if old != nil {
		if idx := strings.LastIndex(old.KID, ":v"); idx != -1 {
			if v, err := strconv.Atoi(old.KID[idx+2:]); err == nil {
				version = v + 1
			}
		}
	}
	return fmt.Sprintf("%s:v%d", prefix, version)
}

func baseKeyPrefix(tenantID *uint64) string {
	if tenantID == nil {
		return "t:global"
	}
	return fmt.Sprintf("t:%d", *tenantID)
}
