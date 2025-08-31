package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ---------- 包裹私钥（AES-GCM） ----------
type Wrapped struct {
	WrapAlg string `json:"wrapAlg"` // "AES-GCM"
	Nonce   string `json:"nonce"`   // b64
	CT      string `json:"ct"`      // b64
	TS      int64  `json:"ts"`
}

// ========== 全局主密钥（唯一可信来源；不再读取环境变量） ==========

var (
	globalWrapKeyMu  sync.RWMutex
	globalWrapKeyRaw []byte // 解码后的 32 字节
	globalWrapKeySet bool
	globalWrapKeyB64 string // 仅用于 Get，便于自检
)

// SetGlobalKeyB64 设置全局主密钥（Base64；解码后必须 32 字节）。
// 传入空字符串表示清空全局密钥（之后 Wrap/Unwrap 会报错，提醒先 Set）。
func SetGlobalKeyB64(b64 string) error {
	if b64 == "" {
		globalWrapKeyMu.Lock()
		globalWrapKeyRaw = nil
		globalWrapKeySet = false
		globalWrapKeyB64 = ""
		globalWrapKeyMu.Unlock()
		return nil
	}
	dec, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return fmt.Errorf("SetGlobalKeyB64: invalid base64: %w", err)
	}
	if len(dec) != 32 {
		return fmt.Errorf("SetGlobalKeyB64: wrap key must be 32 bytes, got %d", len(dec))
	}
	// 拷贝到私有缓冲，避免外部修改底层切片
	tmp := make([]byte, 32)
	copy(tmp, dec)

	globalWrapKeyMu.Lock()
	globalWrapKeyRaw = tmp
	globalWrapKeySet = true
	globalWrapKeyB64 = b64
	globalWrapKeyMu.Unlock()
	return nil
}

// GetGlobalKeyB64 返回当前全局主密钥的 Base64（若未设置则为空）。
func GetGlobalKeyB64() string {
	globalWrapKeyMu.RLock()
	s := globalWrapKeyB64
	globalWrapKeyMu.RUnlock()
	return s
}

// 别名（按你的命名要求）
func GetetGlobalKeyB64() string { return GetGlobalKeyB64() }

// 仅从全局获取；未设置则报错。
// 第二个返回值 kid 固定为 "global"（仅用于日志/审计，不参与加密）。
func loadWrapKey() ([]byte, string, error) {
	globalWrapKeyMu.RLock()
	ok := globalWrapKeySet && len(globalWrapKeyRaw) == 32
	if ok {
		raw := make([]byte, 32)
		copy(raw, globalWrapKeyRaw)
		globalWrapKeyMu.RUnlock()
		return raw, "global", nil
	}
	globalWrapKeyMu.RUnlock()
	return nil, "", errors.New("missing GLOBAL_WRAP_MASTER_KEY (call crypto.SetGlobalKeyB64 at bootstrap)")
}

// ---------- 包裹/解包（使用全局密钥） ----------

func WrapPrivateKey(pemPriv []byte) (*Wrapped, error) {
	key, _, err := loadWrapKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := aead.Seal(nil, nonce, pemPriv, nil)
	return &Wrapped{
		WrapAlg: "AES-GCM",
		Nonce:   base64.StdEncoding.EncodeToString(nonce),
		CT:      base64.StdEncoding.EncodeToString(ct),
		TS:      time.Now().Unix(),
	}, nil
}

func UnwrapPrivateKey(w *Wrapped) ([]byte, error) {
	key, _, err := loadWrapKey()
	if err != nil {
		return nil, err
	}
	nonce, err := base64.StdEncoding.DecodeString(w.Nonce)
	if err != nil {
		return nil, err
	}
	ct, err := base64.StdEncoding.DecodeString(w.CT)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, ct, nil)
}

// ---------- 允许直接传入 32B 密钥的 With 系列（不依赖全局） ----------

func WrapPrivateKeyWith(id string, key32 []byte, pemPriv []byte) (*Wrapped, error) {
	if len(key32) != 32 {
		return nil, errors.New("wrap key must be 32 bytes")
	}
	block, err := aes.NewCipher(key32)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := aead.Seal(nil, nonce, pemPriv, nil)

	return &Wrapped{
		WrapAlg: "AES-GCM",
		Nonce:   base64.StdEncoding.EncodeToString(nonce),
		CT:      base64.StdEncoding.EncodeToString(ct),
		TS:      time.Now().Unix(),
	}, nil
}

func UnwrapPrivateKeyWith(id string, key32 []byte, w *Wrapped) ([]byte, error) {
	if len(key32) != 32 {
		return nil, errors.New("wrap key must be 32 bytes")
	}
	nonce, err := base64.StdEncoding.DecodeString(w.Nonce)
	if err != nil {
		return nil, err
	}
	ct, err := base64.StdEncoding.DecodeString(w.CT)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key32)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, ct, nil)
}

// ---------- 生成 RSA 密钥对（PEM） ----------
func GenerateRSA() (pubPEM, privPEM []byte, err error) {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	privBytes := x509.MarshalPKCS1PrivateKey(k)
	privPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})
	pubBytes := x509.MarshalPKCS1PublicKey(&k.PublicKey)
	pubPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: pubBytes})
	return
}

// ---------- 用公钥加密 / 私钥解密 ----------
func EncryptWithPublicPEM(pubPEM []byte, plaintext []byte) (string, error) {
	block, _ := pem.Decode(pubPEM)
	if block == nil || block.Type != "RSA PUBLIC KEY" {
		return "", errors.New("invalid public pem")
	}
	pub, err := x509.ParsePKCS1PublicKey(block.Bytes)
	if err != nil {
		return "", err
	}
	ct, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, pub, plaintext, nil)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ct), nil
}

func DecryptWithPrivatePEM(privPEM []byte, b64ct string) ([]byte, error) {
	block, _ := pem.Decode(privPEM)
	if block == nil || block.Type != "RSA PRIVATE KEY" {
		return nil, errors.New("invalid private pem")
	}
	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ct, err := base64.StdEncoding.DecodeString(b64ct)
	if err != nil {
		return nil, err
	}
	return rsa.DecryptOAEP(sha256.New(), rand.Reader, priv, ct, nil)
}

// ---------- 封装到 Data["__sealed"] ----------
type Sealed struct {
	Alg string `json:"alg"` // "RSA-OAEP-256"
	KID string `json:"kid"`
	CT  string `json:"ct"` // b64
}

func SealJSONWithPub(pubPEM []byte, kid string, payload any) (*Sealed, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	ct, err := EncryptWithPublicPEM(pubPEM, b)
	if err != nil {
		return nil, err
	}
	return &Sealed{Alg: "RSA-OAEP-256", KID: kid, CT: ct}, nil
}

func UnsealJSONWithPriv(privPEM []byte, sealed *Sealed, out any) error {
	plain, err := DecryptWithPrivatePEM(privPEM, sealed.CT)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(plain))
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}
