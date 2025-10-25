package authorization

import (
	"context"
	"encoding/base64"
	"errors"
	"sync"
	"time"

	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

var (
	// ErrSecretsUnavailable 表示密钥管理器未启用。
	ErrSecretsUnavailable = errors.New("authorization.secrets: unavailable")
	// ErrSecretsRotationDisabled 表示当前配置未启用自动轮换。
	ErrSecretsRotationDisabled = errors.New("authorization.secrets: rotation disabled")
)

// KMSClient 抽象外部 KMS 能力，便于替换实现。
type KMSClient interface {
	Encrypt(ctx context.Context, keyID string, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, keyID string, ciphertext []byte) ([]byte, error)
	GenerateDataKey(ctx context.Context, keyID string) (DataKey, error)
	RotateKey(ctx context.Context, keyID string) (string, error)
}

// DataKey 描述通过 KMS 生成的数据密钥。
type DataKey struct {
	Plaintext  []byte
	Ciphertext []byte
	KeyID      string
}

// SecretsManagerOptions 控制密钥管理行为。
type SecretsManagerOptions struct {
	Client           KMSClient
	KeyID            string
	RotationInterval time.Duration
	CacheTTL         time.Duration
	Logger           *pxlog.Logger
	Now              func() time.Time
}

// SecretsManager 为授权域提供敏感配置的加解密与轮换辅助。
type SecretsManager struct {
	client           KMSClient
	keyID            string
	rotationInterval time.Duration
	cacheTTL         time.Duration
	logger           *pxlog.Logger
	now              func() time.Time

	mu            sync.RWMutex
	cachedKey     *cachedKey
	lastRotatedAt time.Time
}

type cachedKey struct {
	plaintext []byte
	expireAt  time.Time
	keyID     string
}

// NewSecretsManager 构建 SecretsManager。
func NewSecretsManager(opts SecretsManagerOptions) *SecretsManager {
	logger := opts.Logger
	if logger == nil {
		logger = pxlog.GetGlobalLogger()
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	cacheTTL := opts.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = 30 * time.Minute
	}
	return &SecretsManager{
		client:           opts.Client,
		keyID:            opts.KeyID,
		rotationInterval: opts.RotationInterval,
		cacheTTL:         cacheTTL,
		logger:           logger,
		now:              now,
	}
}

// Enabled 返回密钥管理器是否可用。
func (m *SecretsManager) Enabled() bool {
	return m != nil && m.client != nil && m.keyID != ""
}

// EncryptConfig 使用 KMS 加密敏感配置。
func (m *SecretsManager) EncryptConfig(ctx context.Context, plaintext []byte) ([]byte, error) {
	if !m.Enabled() {
		return nil, ErrSecretsUnavailable
	}
	cipher, err := m.client.Encrypt(ctx, m.keyID, plaintext)
	if err != nil {
		return nil, err
	}
	return []byte(base64.StdEncoding.EncodeToString(cipher)), nil
}

// DecryptConfig 解密敏感配置内容。
func (m *SecretsManager) DecryptConfig(ctx context.Context, ciphertext []byte) ([]byte, error) {
	if !m.Enabled() {
		return nil, ErrSecretsUnavailable
	}
	raw := make([]byte, base64.StdEncoding.DecodedLen(len(ciphertext)))
	n, err := base64.StdEncoding.Decode(raw, ciphertext)
	if err != nil {
		return nil, err
	}
	raw = raw[:n]

	plaintext, err := m.client.Decrypt(ctx, m.keyID, raw)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

// CurrentDataKey 返回缓存的数据密钥，如缓存失效则向 KMS 申请新的数据密钥。
func (m *SecretsManager) CurrentDataKey(ctx context.Context) (DataKey, error) {
	if !m.Enabled() {
		return DataKey{}, ErrSecretsUnavailable
	}

	if cached := m.loadCachedKey(); cached != nil {
		return DataKey{
			Plaintext: append([]byte(nil), cached.plaintext...),
			KeyID:     cached.keyID,
		}, nil
	}

	return m.refreshDataKey(ctx)
}

// RotateIfDue 当达到轮换间隔时，调用 KMS 轮换主密钥。
func (m *SecretsManager) RotateIfDue(ctx context.Context) error {
	if !m.Enabled() {
		return ErrSecretsUnavailable
	}
	if m.rotationInterval <= 0 {
		return ErrSecretsRotationDisabled
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	nextRotation := m.lastRotatedAt.Add(m.rotationInterval)
	if m.lastRotatedAt.IsZero() || m.now().After(nextRotation) {
		newKeyID, err := m.client.RotateKey(ctx, m.keyID)
		if err != nil {
			return err
		}
		if newKeyID != "" && newKeyID != m.keyID {
			m.keyID = newKeyID
		}
		m.lastRotatedAt = m.now()
		m.cachedKey = nil
	}
	return nil
}

func (m *SecretsManager) loadCachedKey() *cachedKey {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cachedKey == nil || m.cachedKey.expireAt.Before(m.now()) {
		return nil
	}
	return m.cachedKey
}

func (m *SecretsManager) refreshDataKey(ctx context.Context) (DataKey, error) {
	key, err := m.client.GenerateDataKey(ctx, m.keyID)
	if err != nil {
		return DataKey{}, err
	}
	m.mu.Lock()
	m.cachedKey = &cachedKey{
		plaintext: append([]byte(nil), key.Plaintext...),
		expireAt:  m.now().Add(m.cacheTTL),
		keyID:     key.KeyID,
	}
	m.mu.Unlock()
	return DataKey{
		Plaintext:  key.Plaintext,
		Ciphertext: key.Ciphertext,
		KeyID:      key.KeyID,
	}, nil
}

// NewNoopKMSClient 提供一个默认的空实现，主要用于本地开发环境。
func NewNoopKMSClient() KMSClient {
	return noopKMS{}
}

type noopKMS struct{}

func (noopKMS) Encrypt(_ context.Context, _ string, plaintext []byte) ([]byte, error) {
	return append([]byte(nil), plaintext...), nil
}

func (noopKMS) Decrypt(_ context.Context, _ string, ciphertext []byte) ([]byte, error) {
	return append([]byte(nil), ciphertext...), nil
}

func (noopKMS) GenerateDataKey(_ context.Context, keyID string) (DataKey, error) {
	data := []byte("noop-key-material")
	return DataKey{
		Plaintext:  append([]byte(nil), data...),
		Ciphertext: append([]byte(nil), data...),
		KeyID:      keyID,
	}, nil
}

func (noopKMS) RotateKey(_ context.Context, keyID string) (string, error) {
	return keyID, nil
}
