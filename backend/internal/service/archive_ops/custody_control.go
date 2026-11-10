package archive_ops

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

// CustodyStatus 区分原生任务与旧脚本实际接管，页面不能把两个开关混为一谈。
type CustodyStatus struct {
	Protocol       int    `json:"protocol"`
	NodeID         string `json:"node_id"`
	LegacyDetected bool   `json:"legacy_detected"`
	Managed        bool   `json:"managed"`
	Problem        string `json:"problem,omitempty"`
}
type custodySource struct {
	SourceID       string `json:"source_id"`
	Directory      string `json:"directory"`
	Executor       string `json:"executor"`
	ArchiveEnabled bool   `json:"archive_enabled"`
}
type custodyControl struct {
	Protocol    int             `json:"protocol"`
	NodeID      string          `json:"node_id"`
	PublishedAt time.Time       `json:"published_at"`
	ValidUntil  time.Time       `json:"valid_until"`
	Sources     []custodySource `json:"sources"`
}
type custodyBinding struct {
	Protocol     int      `json:"protocol"`
	NodeID       string   `json:"node_id"`
	ControlFile  string   `json:"control_file"`
	CoreUID      uint32   `json:"core_uid"`
	Sources      []string `json:"sources"`
	GuardScript  string   `json:"guard_script"`
	GuardSHA256  string   `json:"guard_sha256"`
	HelperScript string   `json:"helper_script"`
	HelperSHA256 string   `json:"helper_sha256"`
	PolicyScript string   `json:"policy_script"`
	PolicySHA256 string   `json:"policy_sha256"`
}

func (s *Service) publishCustodyControl() error {
	now := time.Now().UTC()
	out := custodyControl{Protocol: 2, NodeID: s.cfg.ExecutionNodeID, PublishedAt: now, ValidUntil: now.Add(3 * time.Minute), Sources: []custodySource{}}
	ids, err := s.policyIDs(s.ctx)
	if err != nil {
		if s.policyControlRequired() {
			ids = nil
		} else {
			return err
		}
	}
	enabled := map[string]bool{}
	for _, id := range ids {
		p, err := s.policy(s.ctx, id)
		if err != nil {
			if s.policyControlRequired() {
				continue
			}
			return err
		}
		if p.Enabled && p.TargetID != "" {
			t, err := s.target(p.TargetID)
			if err == nil && s.targetVerified(t) && p.TargetRevision == t.Revision && p.TargetFingerprint == t.VerifiedFingerprint {
				enabled[p.SourceID] = true
			}
		}
	}
	for _, source := range s.cfg.Sources {
		if isLogSource(source.Kind) {
			out.Sources = append(out.Sources, custodySource{SourceID: source.ID, Directory: source.Directory, Executor: "powerx", ArchiveEnabled: enabled[source.ID]})
		}
	}
	return writeJSON(filepath.Join(s.cfg.Directory, "custody-control.json"), out)
}
func (s *Service) custodyBinding() (custodyBinding, error) {
	var b custodyBinding
	p := s.cfg.CustodyBindingFile
	if p == "" && runtime.GOOS == "linux" {
		p = "/etc/powerx-space-guard/core-binding.json"
	}
	st, err := os.Lstat(p)
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0022 != 0 || st.Size() > 16384 {
		return b, errors.New("临时守护尚未完成只读绑定")
	}
	stat, ok := st.Sys().(*syscall.Stat_t)
	owner := uint32(0)
	if runtime.GOOS != "linux" {
		owner = uint32(os.Geteuid())
	}
	if !ok || stat.Uid != owner || stat.Nlink != 1 {
		return b, errors.New("临时守护绑定属主或链接不可信")
	}
	raw, err := os.ReadFile(p)
	if err != nil || json.Unmarshal(raw, &b) != nil || b.Protocol != 2 || b.NodeID != s.cfg.ExecutionNodeID || b.CoreUID != uint32(os.Geteuid()) || b.ControlFile != filepath.Join(s.cfg.Directory, "custody-control.json") {
		return b, errors.New("临时守护绑定与当前实例不匹配")
	}
	for path, want := range map[string]string{b.GuardScript: b.GuardSHA256, b.HelperScript: b.HelperSHA256, b.PolicyScript: b.PolicySHA256} {
		st, err := os.Lstat(path)
		if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0022 != 0 || st.Size() > 1<<20 {
			return b, errors.New("临时守护程序不可验证")
		}
		meta, ok := st.Sys().(*syscall.Stat_t)
		if !ok || meta.Uid != owner || meta.Nlink != 1 {
			return b, errors.New("临时守护程序权限不可信")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return b, err
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != want {
			return b, errors.New("临时守护程序版本变化，需要重新接管")
		}
	}
	return b, nil
}
func (s *Service) custodyStatus() CustodyStatus {
	out := CustodyStatus{Protocol: 2, NodeID: s.cfg.ExecutionNodeID}
	_, err := os.Stat("/opt/powerx-ops/space-guard/audit_custody.py")
	out.LegacyDetected = err == nil || s.cfg.CustodyBindingFile != ""
	if !out.LegacyDetected {
		return out
	}
	_, err = s.custodyBinding()
	out.Managed = err == nil
	if err != nil {
		out.Problem = "legacy_handoff_required"
	}
	return out
}

// ManagedLegacySource 只接受部署方固定目录和已核对程序，不以 UI 的显示状态代替接管。
func (s *Service) ManagedLegacySource(directory string) bool {
	if s == nil || s.initErr != nil {
		return false
	}
	b, err := s.custodyBinding()
	if err != nil {
		return false
	}
	registered := false
	for _, source := range s.cfg.Sources {
		if source.Directory == directory && isLogSource(source.Kind) {
			registered = true
			break
		}
	}
	if !registered {
		return false
	}
	for _, path := range b.Sources {
		if path == directory {
			return true
		}
	}
	return false
}
