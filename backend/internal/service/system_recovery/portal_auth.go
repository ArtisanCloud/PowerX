package system_recovery

// Recovery authentication is deliberately outside the restored database and
// application configuration. A stopped Core cannot issue or revoke this session.
import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/google/uuid"
)

const recoveryCookie = "powerx_recovery_admin"
const recoverySessionTTL = 2 * time.Hour

type recoverySession struct {
	ID        string    `json:"id"`
	Digest    string    `json:"digest"`
	Operator  string    `json:"operator"`
	Method    string    `json:"method"`
	CSRF      string    `json:"csrf"`
	ExpiresAt time.Time `json:"expires_at"`
}

func recoveryRandom() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
func (c *Coordinator) recoveryAuthDirectory() string {
	return filepath.Join(c.Store.Directory, "recovery-auth")
}
func (c *Coordinator) recoveryKeyFile() string {
	return filepath.Join(c.recoveryAuthDirectory(), "administrator.key")
}

// EnsureRecoveryAccess provisions one private random key per installation.
// It returns only its filename; secrets must never appear in logs or CLI args.
func (c *Coordinator) EnsureRecoveryAccess() (string, error) {
	for _, path := range []string{c.recoveryAuthDirectory(), filepath.Join(c.recoveryAuthDirectory(), "sessions"), filepath.Join(c.recoveryAuthDirectory(), "audit")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			return "", err
		}
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
			return "", errors.New("恢复认证目录必须是私有普通目录")
		}
	}
	file := c.recoveryKeyFile()
	if _, err := os.Lstat(file); errors.Is(err, os.ErrNotExist) {
		key, err := recoveryRandom()
		if err != nil {
			return "", err
		}
		out, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			_, err = out.WriteString("pxr_" + key + "\n")
			if err == nil {
				err = out.Sync()
			}
			closeErr := out.Close()
			if err != nil {
				_ = os.Remove(file)
				return "", err
			}
			if closeErr != nil {
				return "", closeErr
			}
		} else if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	if _, err := c.readRecoveryKey(); err != nil {
		return "", err
	}
	return file, nil
}
func (c *Coordinator) readRecoveryKey() (string, error) {
	info, err := os.Lstat(c.recoveryKeyFile())
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 128 {
		return "", errors.New("恢复管理员密钥不可用")
	}
	raw, err := os.ReadFile(c.recoveryKeyFile())
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(string(raw))
	if len(key) != 68 || !strings.HasPrefix(key, "pxr_") {
		return "", errors.New("恢复管理员密钥格式无效")
	}
	if _, err = hex.DecodeString(key[4:]); err != nil {
		return "", errors.New("恢复管理员密钥格式无效")
	}
	return key, nil
}
func (c *Coordinator) verifyRecoveryKey(key string) bool {
	stored, err := c.readRecoveryKey()
	if err != nil {
		return false
	}
	a, b := sha256.Sum256([]byte(stored)), sha256.Sum256([]byte(key))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}
func (c *Coordinator) sessionFile(id string) string {
	return filepath.Join(c.recoveryAuthDirectory(), "sessions", id+".json")
}
func (c *Coordinator) readRecoverySession(r *http.Request) (recoverySession, error) {
	var session recoverySession
	cookie, err := r.Cookie(recoveryCookie)
	if err != nil {
		return session, errors.New("请验证恢复管理员身份")
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 || uuid.Validate(parts[0]) != nil || len(parts[1]) != 64 {
		return session, errors.New("恢复会话无效")
	}
	info, err := os.Lstat(c.sessionFile(parts[0]))
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return session, errors.New("恢复会话已失效，请重新验证")
	}
	if err = snapshots.ReadCoordinatorJSON(c.sessionFile(parts[0]), &session); err != nil {
		return session, err
	}
	digest := sha256.Sum256([]byte(parts[1]))
	if session.ID != parts[0] || session.Operator == "" || session.CSRF == "" || time.Now().After(session.ExpiresAt) || subtle.ConstantTimeCompare([]byte(hex.EncodeToString(digest[:])), []byte(session.Digest)) != 1 {
		return recoverySession{}, errors.New("恢复会话已失效，请重新验证")
	}
	return session, nil
}
func (c *Coordinator) issueRecoverySession(w http.ResponseWriter, r *http.Request, operator, method string) (recoverySession, error) {
	if _, err := c.EnsureRecoveryAccess(); err != nil {
		return recoverySession{}, err
	}
	if prior, err := c.readRecoverySession(r); err == nil && prior.Operator == operator && prior.Method == method && time.Until(prior.ExpiresAt) > 15*time.Minute {
		return prior, nil
	}
	secret, err := recoveryRandom()
	if err != nil {
		return recoverySession{}, err
	}
	csrf, err := recoveryRandom()
	if err != nil {
		return recoverySession{}, err
	}
	digest := sha256.Sum256([]byte(secret))
	session := recoverySession{ID: uuid.NewString(), Digest: hex.EncodeToString(digest[:]), Operator: operator, Method: method, CSRF: csrf, ExpiresAt: time.Now().UTC().Add(recoverySessionTTL)}
	if err = snapshots.WriteCoordinatorJSON(c.sessionFile(session.ID), session); err != nil {
		return recoverySession{}, err
	}
	http.SetCookie(w, &http.Cookie{Name: recoveryCookie, Value: session.ID + "." + secret, Path: "/", HttpOnly: true, Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https", SameSite: http.SameSiteStrictMode, MaxAge: int(recoverySessionTTL.Seconds()), Expires: session.ExpiresAt})
	c.portalAudit("session.created", session.Operator, method)
	return session, nil
}
func recoverySameOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host == r.Host && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" && parsed.Path == ""
}
func recoveryCSRF(r *http.Request, session recoverySession) bool {
	return recoverySameOrigin(r) && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-PowerX-Recovery-CSRF")), []byte(session.CSRF)) == 1
}
func (c *Coordinator) portalAudit(action, operator, detail string) {
	_ = snapshots.WriteCoordinatorJSON(filepath.Join(c.recoveryAuthDirectory(), "audit", uuid.NewString()+".json"), map[string]any{"at": time.Now().UTC(), "action": action, "operator": operator, "detail": detail})
}

func (c *Coordinator) allowPortalLogin() bool {
	c.portalLoginMu.Lock()
	defer c.portalLoginMu.Unlock()
	cutoff := time.Now().Add(-time.Minute)
	kept := c.portalLogins[:0]
	for _, at := range c.portalLogins {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	c.portalLogins = kept
	if len(kept) >= 20 {
		return false
	}
	c.portalLogins = append(c.portalLogins, time.Now())
	return true
}
