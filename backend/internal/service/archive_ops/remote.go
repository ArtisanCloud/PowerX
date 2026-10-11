package archive_ops

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed receiver.py
var receiverScript string

func quoteShell(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
func (s *Service) sshArgs(t Target) ([]string, error) {
	c, ok := s.cfg.Credentials[t.CredentialRef]
	if !ok {
		return nil, errors.New("SSH 凭据引用不可用")
	}
	for _, path := range []string{c.KeyFile, c.KnownHostsFile} {
		info, e := os.Lstat(path)
		if e != nil || !info.Mode().IsRegular() {
			return nil, errors.New("SSH 私钥或 known_hosts 文件不可读")
		}
	}
	info, _ := os.Stat(c.KeyFile)
	if info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("SSH 私钥权限必须是 0600 或更严格")
	}
	return []string{"-F", "/dev/null", "-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes", "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=" + c.KnownHostsFile, "-o", "ConnectTimeout=10", "-i", c.KeyFile, "-p", strconv.Itoa(t.Port)}, nil
}
func archiveCommand(ctx context.Context, program string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = 5 * time.Second
	return cmd
}
func (s *Service) remote(ctx context.Context, t Target, op, policyID, jobID string, params map[string]any) (map[string]any, error) {
	args, e := s.sshArgs(t)
	if e != nil {
		return nil, e
	}
	remoteArgs := []string{"python3", "-", t.Directory, s.cfg.Instance, policyID, jobID, op}
	if op == "commit" {
		remoteArgs = append(remoteArgs, fmt.Sprint(params["days"]), fmt.Sprint(params["count"]))
	}
	if op == "protect" {
		remoteArgs = append(remoteArgs, fmt.Sprint(params["protected"]))
	}
	quoted := []string{}
	for _, arg := range remoteArgs {
		quoted = append(quoted, quoteShell(arg))
	}
	args = append(args, t.User+"@"+t.Host, strings.Join(quoted, " "))
	cmd := archiveCommand(ctx, "ssh", args...)
	cmd.Stdin = strings.NewReader(receiverScript)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if e = cmd.Run(); e != nil {
		return nil, errors.New("远端接收器失败，请检查 SSH 信任、Python3、目录权限及文件校验")
	}
	var result map[string]any
	if json.Unmarshal(out.Bytes(), &result) != nil || result["ok"] != true {
		return nil, errors.New("远端确认格式无效")
	}
	return result, nil
}
func (s *Service) rsync(ctx context.Context, t Target, from, to string, pull bool) error {
	args, e := s.sshArgs(t)
	if e != nil {
		return e
	}
	ssh := []string{"ssh"}
	ssh = append(ssh, args...)
	for i := range ssh {
		ssh[i] = quoteShell(ssh[i])
	}
	host := t.Host
	if net.ParseIP(host) != nil && strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	remote := t.User + "@" + host + ":" + to
	values := []string{"-r", "--partial", "--checksum", "-e", strings.Join(ssh, " ")}
	if pull {
		values = append(values, remote, from)
	} else {
		values = append(values, from, remote)
	}
	cmd := archiveCommand(ctx, "rsync", values...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if e = cmd.Run(); e != nil {
		return errors.New("rsync 传输失败，请检查连接、磁盘空间及远端 rsync 安装")
	}
	return nil
}
func (s *Service) upload(ctx context.Context, j *jobRecord) {
	j.Attempts++
	j.RemoteState = "pending"
	j.RemoteError = ""
	_ = s.saveJob(j)
	e := s.uploadSnapshot(ctx, j)
	if e != nil {
		j.RemoteState = "failed"
		j.RemoteError = e.Error()
		delay := time.Duration(1<<min(j.Attempts, 8)) * time.Minute
		j.RetryAt = time.Now().UTC().Add(delay)
	} else {
		j.RemoteState = "success"
		j.RemoteError = ""
		j.RetryAt = time.Time{}
	}
	_ = s.saveJob(j)
}
func (s *Service) uploadSnapshot(ctx context.Context, j *jobRecord) error {
	t := *j.Target
	result, e := s.remote(ctx, t, "init", j.PolicyID, j.ID, nil)
	if e != nil {
		return e
	}
	namespace, ok := result["namespace"].(string)
	expected := namespace
	if !ok || result["configured_root"] != t.Directory || !filepath.IsAbs(namespace) || !strings.HasSuffix(namespace, "/"+s.cfg.Instance+"/"+j.PolicyID) {
		return errors.New("远端目录身份不符")
	}
	stage := filepath.Join(s.cfg.Directory, "staging", j.ID)
	if e = os.Mkdir(stage, 0700); e != nil {
		return e
	}
	defer os.RemoveAll(stage)
	for _, entry := range j.Files {
		if !shaPattern.MatchString(entry.SHA256) {
			return errors.New("对象清单无效")
		}
		source := s.objectPath(entry.SHA256)
		if e = verifyObject(ctx, source, entry); e != nil {
			return e
		}
		path := filepath.Join(stage, "objects", entry.SHA256[:2], entry.SHA256)
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return e
		}
		if _, e = os.Lstat(path); os.IsNotExist(e) {
			if e = os.Link(source, path); e != nil {
				return e
			}
		}
	}
	if e = s.rsync(ctx, t, filepath.Join(stage, "objects")+"/", expected+"/objects/", false); e != nil {
		return e
	}
	// 验证后的快照清单最后发布，未完成传输不会成为可恢复归档。
	manifestPath := filepath.Join(stage, j.ID+".json")
	if e = writeJSON(manifestPath, j.Job); e != nil {
		return e
	}
	if e = s.rsync(ctx, t, manifestPath, expected+"/incoming/"+j.ID+".json", false); e != nil {
		return e
	}
	if _, e = s.remote(ctx, t, "commit", j.PolicyID, j.ID, map[string]any{"days": j.RemoteRetentionDays, "count": j.RemoteRetentionCount}); e != nil {
		return e
	}
	j.RemotePath = expected + "/manifests/" + j.ID + ".json"
	return nil
}
func (s *Service) TestTarget(ctx context.Context, id string) error {
	if e := s.check(ctx); e != nil {
		return e
	}
	t, e := s.target(id)
	if e != nil {
		return e
	}
	testID := "00000000-0000-4000-8000-000000000000"
	_, e = s.remote(ctx, t, "init", testID, "-", nil)
	return e
}

// 已按本地策略清理的备份仍可从远端下载；必须匹配本机保留的清单。
func (s *Service) ensureObjects(ctx context.Context, j jobRecord) error {
	available := true
	for _, entry := range j.Files {
		if !shaPattern.MatchString(entry.SHA256) {
			return errors.New("对象清单无效")
		}
		if verifyObject(ctx, s.objectPath(entry.SHA256), entry) != nil {
			available = false
			break
		}
	}
	if available {
		return nil
	}
	if j.Target == nil || j.RemoteState != "success" {
		return errors.New("本地对象已不可用，且没有已验证异地副本")
	}
	result, e := s.remote(ctx, *j.Target, "read", j.PolicyID, j.ID, nil)
	if e != nil {
		return e
	}
	raw, e := json.Marshal(result["manifest"])
	if e != nil {
		return e
	}
	var remoteJob Job
	if json.Unmarshal(raw, &remoteJob) != nil || remoteJob.ID != j.ID || remoteJob.PolicyID != j.PolicyID {
		return errors.New("远端清单身份不符")
	}
	expected := map[string]Entry{}
	for _, f := range j.Files {
		expected[f.Path] = f
	}
	if len(expected) != len(remoteJob.Files) {
		return errors.New("远端文件清单变化")
	}
	for _, f := range remoteJob.Files {
		want, ok := expected[f.Path]
		if !ok || want.SHA256 != f.SHA256 || want.Size != f.Size {
			return errors.New("远端文件清单校验不符")
		}
	}
	namespace, ok := result["namespace"].(string)
	if !ok || result["configured_root"] != j.Target.Directory || !strings.HasSuffix(namespace, "/"+s.cfg.Instance+"/"+j.PolicyID) {
		return errors.New("远端目录身份不符")
	}
	for _, entry := range j.Files {
		dest := s.objectPath(entry.SHA256)
		if verifyObject(ctx, dest, entry) == nil {
			continue
		}
		if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
			return e
		}
		if e = s.rsync(ctx, *j.Target, filepath.Dir(dest)+"/", namespace+"/objects/"+entry.SHA256[:2]+"/"+entry.SHA256, true); e != nil {
			return e
		}
		if e = verifyObject(ctx, dest, entry); e != nil {
			return e
		}
	}
	return nil
}
