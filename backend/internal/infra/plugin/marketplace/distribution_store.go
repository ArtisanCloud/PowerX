package marketplace

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/infra/plugin/runtimecredential"
	"github.com/ArtisanCloud/PowerX/pkg/instanceidentity"
	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

// DistributionStore 是安装环境的本地交付日志，不属于业务租户或插件版本目录。
// 文件按市场与环境分区，进程锁阻止两个进程共同消费同一份日志。
type DistributionStore struct {
	mu    sync.Mutex
	file  string
	lock  *os.File
	state DistributionState
}
type DistributionState struct {
	Protocol             string                     `json:"protocol"`
	SourceID             string                     `json:"source_id"`
	EnvironmentID        string                     `json:"environment_id"`
	Fingerprint          string                     `json:"fingerprint"`
	InstanceID           string                     `json:"instance_id,omitempty"`
	InstanceToken        string                     `json:"instance_token,omitempty"`
	PendingInstanceToken string                     `json:"pending_instance_token,omitempty"`
	EnrollmentHash       string                     `json:"enrollment_hash,omitempty"`
	RegistrationError    string                     `json:"registration_error,omitempty"`
	LastHeartbeat        time.Time                  `json:"last_heartbeat,omitempty"`
	AuthRejected         bool                       `json:"auth_rejected,omitempty"`
	Attempts             map[string]*InstallAttempt `json:"attempts"`
}
type InstallAttempt struct {
	ID            string          `json:"id"`
	InstanceID    string          `json:"instance_id"`
	PluginID      string          `json:"plugin_id"`
	Version       string          `json:"version"`
	Tenant        string          `json:"tenant"`
	Enable        bool            `json:"enable"`
	PriorRevision string          `json:"prior_revision,omitempty"`
	Revision      string          `json:"revision,omitempty"`
	DownloadID    string          `json:"download_id,omitempty"`
	Target        Target          `json:"target"`
	Phase         string          `json:"phase"`
	CreatedAt     time.Time       `json:"created_at"`
	Events        []PendingReport `json:"events"`
	Failures      int             `json:"failures"`
	NextAttempt   time.Time       `json:"next_attempt,omitempty"`
	Blocked       string          `json:"blocked,omitempty"`
	NeedsReview   bool            `json:"needs_review,omitempty"`
}
type PendingReport struct {
	Payload TransferReport `json:"payload"`
	Sent    bool           `json:"sent"`
}

func Fingerprint(environmentID string) string {
	sum := sha256.Sum256([]byte("powerx:marketplace:instance:v1:" + environmentID))
	return hex.EncodeToString(sum[:])
}
func OpenDistributionStore(directory, sourceID, environmentID string) (*DistributionStore, error) {
	if directory == "" || sourceID == "" || !instanceidentity.Valid(environmentID) {
		return nil, fmt.Errorf("Marketplace 持久化需要有效的环境身份和市场地址")
	}
	sum := sha256.Sum256([]byte(sourceID + "\n" + environmentID))
	directory = filepath.Join(directory, hex.EncodeToString(sum[:]))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("Marketplace 私有目录不可写")
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("Marketplace 私有目录无效")
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return nil, fmt.Errorf("Marketplace 私有目录权限不可设置")
	}
	lockPath := filepath.Join(directory, "instance.lock")
	if info, err := os.Lstat(lockPath); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("Marketplace 进程锁文件无效")
	}
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("Marketplace 进程锁不可写")
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("此环境的 Marketplace 日志正被另一 PowerX 进程使用")
	}
	s := &DistributionStore{file: filepath.Join(directory, "distribution.json"), lock: lock, state: DistributionState{
		Protocol: "powerx.marketplace-distribution/v1", SourceID: sourceID, EnvironmentID: environmentID, Fingerprint: Fingerprint(environmentID), Attempts: map[string]*InstallAttempt{},
	}}
	if info, err := os.Lstat(s.file); err == nil {
		if !info.Mode().IsRegular() || info.Size() > 32<<20 {
			_ = s.Close()
			return nil, fmt.Errorf("Marketplace 日志无效")
		}
		file, err := os.Open(s.file)
		if err != nil {
			_ = s.Close()
			return nil, fmt.Errorf("Marketplace 日志不可读")
		}
		err = json.NewDecoder(io.LimitReader(file, (32<<20)+1)).Decode(&s.state)
		_ = file.Close()
		if err != nil || s.state.Protocol != "powerx.marketplace-distribution/v1" || s.state.SourceID != sourceID || s.state.EnvironmentID != environmentID || s.state.Fingerprint != Fingerprint(environmentID) || s.state.Attempts == nil {
			_ = s.Close()
			return nil, fmt.Errorf("Marketplace 日志与当前环境不匹配或已损坏")
		}
		if (s.state.InstanceID == "") != (s.state.InstanceToken == "") {
			_ = s.Close()
			return nil, fmt.Errorf("Marketplace 实例凭证不完整，请核对并重新接入")
		}
		if token := s.state.PendingInstanceToken; token != "" {
			raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, "pxm_instance_"))
			if !strings.HasPrefix(token, "pxm_instance_") || len(token) != len("pxm_instance_")+43 || err != nil || len(raw) != 32 {
				_ = s.Close()
				return nil, fmt.Errorf("Marketplace 待登记凭证已损坏，保留原记录等待核对")
			}
		}
		if s.state.InstanceID != "" {
			if _, err := uuid.Parse(s.state.InstanceID); err != nil {
				_ = s.Close()
				return nil, fmt.Errorf("Marketplace 实例记录无效")
			}
		}
		for id, a := range s.state.Attempts {
			if a == nil || a.ID != id {
				_ = s.Close()
				return nil, fmt.Errorf("Marketplace 安装尝试记录无效")
			}
		}
		if err := os.Chmod(s.file, 0o600); err != nil {
			_ = s.Close()
			return nil, fmt.Errorf("Marketplace 日志权限不可设置")
		}
	} else if !os.IsNotExist(err) {
		_ = s.Close()
		return nil, fmt.Errorf("Marketplace 日志不可读")
	}
	return s, nil
}
func (s *DistributionStore) Snapshot() DistributionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, _ := json.Marshal(s.state)
	var copy DistributionState
	_ = json.Unmarshal(raw, &copy)
	return copy
}

// Identity 用于鉴权就绪检查，不复制不断增长的交付历史。
func (s *DistributionStore) Identity() DistributionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := s.state
	copy.Attempts = nil
	return copy
}
func (s *DistributionStore) Attempt(id string) *InstallAttempt {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Attempts[id] == nil {
		return nil
	}
	raw, _ := json.Marshal(s.state.Attempts[id])
	var copy InstallAttempt
	_ = json.Unmarshal(raw, &copy)
	return &copy
}

// Update 必须先落盘再更新内存，失败时不会把尚未持久化的结果交给发送器。
func (s *DistributionStore) Update(update func(*DistributionState) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return fmt.Errorf("Marketplace 日志已关闭")
	}
	raw, err := json.Marshal(s.state)
	if err != nil {
		return fmt.Errorf("Marketplace 日志编码失败")
	}
	var next DistributionState
	if err := json.Unmarshal(raw, &next); err != nil {
		return fmt.Errorf("Marketplace 日志复制失败")
	}
	if err := update(&next); err != nil {
		return err
	}
	raw, err = json.Marshal(next)
	if err != nil || len(raw) > 32<<20 {
		return fmt.Errorf("Marketplace 日志过大或编码失败")
	}
	if err := runtimecredential.AtomicWrite(s.file, raw, 0o600); err != nil {
		return fmt.Errorf("Marketplace 日志持久化失败，请检查私有目录")
	}
	s.state = next
	return nil
}
func (s *DistributionStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil
	}
	_ = unix.Flock(int(s.lock.Fd()), unix.LOCK_UN)
	err := s.lock.Close()
	s.lock = nil
	return err
}
