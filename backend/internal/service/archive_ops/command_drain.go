package archive_ops

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
)

// CommandDrainOverview 仅观测本机命令，不认证远端接收器或旧版本执行。
type CommandRecordAttention struct {
	RecordID string `json:"record_id"`
	JobID    string `json:"job_id,omitempty"`
	TargetID string `json:"target_id,omitempty"`
	Name     string `json:"name,omitempty"`
	Subject  string `json:"subject"`
}
type CommandDrainOverview struct {
	Records     []CommandRecordAttention `json:"records,omitempty"`
	Protocol    int                      `json:"protocol"`
	Active      int                      `json:"active"`
	Unconfirmed int                      `json:"unconfirmed"`
	Problem     string                   `json:"problem,omitempty"`
	Jobs        []string                 `json:"jobs,omitempty"`
}
type commandRecord struct {
	Probe       *CommandProbeBinding    `json:"probe,omitempty"`
	Identity    *CommandProcessIdentity `json:"identity,omitempty"`
	Protocol    int                     `json:"protocol"`
	ID          string                  `json:"id"`
	JobID       string                  `json:"job_id,omitempty"`
	ExecutionID string                  `json:"execution_id,omitempty"`
	CommandID   string                  `json:"command_id,omitempty"`
	Kind        string                  `json:"kind,omitempty"`
	State       string                  `json:"state"`
	PID         int                     `json:"pid,omitempty"`
	PGID        int                     `json:"pgid,omitempty"`
	Host        string                  `json:"host"`
	At          time.Time               `json:"at"`
}

func (s *Service) readCommandRecord(id string) (commandRecord, error) {
	var r commandRecord
	if !validID(id) {
		return r, failure(400, "ARCHIVE_ID_INVALID", "命令记录编号无效")
	}
	if e := readJSON(s.path("command-executions", id), &r); e != nil {
		return r, e
	}
	if (r.Protocol != 1 && r.Protocol != 2 && r.Protocol != 3) || r.ID != id || r.JobID != "" && r.JobID != id || !validID(r.ExecutionID) || r.Host == "" || r.At.IsZero() || (r.State != "idle" && r.State != "prepared" && r.State != "running" && r.State != "drained") || (r.State != "idle" && (!validID(r.CommandID) || r.Kind != "pg_dump" && r.Kind != "ssh" && r.Kind != "rsync")) || r.PID < 0 || r.PGID != r.PID || r.State == "running" && r.PID < 1 || r.State == "prepared" && r.PID != 0 || r.State == "idle" && (r.PID != 0 || r.CommandID != "" || r.Kind != "") {
		return r, failure(503, "ARCHIVE_COMMAND_PROOF_INVALID", "本机命令证明损坏，保留内容并停止处置")
	}
	if r.Identity != nil && ((r.Protocol != 2 && r.Protocol != 3) || r.State != "running" && r.State != "drained" || !validCommandIdentity(r.Identity) || r.Identity.PID != r.PID || r.Identity.PGID != r.PGID) {
		return r, failure(503, "ARCHIVE_COMMAND_PROOF_INVALID", "命令启动身份损坏，保留保护")
	}
	if r.Protocol == 3 && (r.JobID != "" || !validProbeBinding(r.Probe, r.ID)) || r.Protocol != 3 && r.Probe != nil {
		return r, failure(503, "ARCHIVE_PROBE_SCOPE_INVALID", "探测绑定损坏，继续保护")
	}
	return r, nil
}
func (s *Service) initializeCommandRecord(id, execution string, job bool, probes ...*CommandProbeBinding) error {
	release, e := s.lock(context.Background(), "command-"+id)
	if e != nil {
		return failure(409, "ARCHIVE_COMMANDS_ACTIVE", "本机命令仍在执行")
	}
	defer release()
	old, e := s.readCommandRecord(id)
	if e == nil && s.commandStateWithoutLock(old) != "drained" {
		return failure(409, "ARCHIVE_COMMAND_DRAIN_UNCONFIRMED", "原本机命令尚未确认排空")
	}
	if e != nil && dto.StatusCode(e) != 404 {
		return e
	}
	registry, e := s.lock(context.Background(), "command-registry")
	if e != nil {
		return ErrBusy
	}
	defer registry()
	ids, e := s.records("command-executions")
	if e != nil {
		return e
	}
	// 仅新增时限制数量；现存任务的序列证明原位更新，不按每个对象无限增长。
	exists := false
	for _, key := range ids {
		if key == id {
			exists = true
		}
	}
	if !exists && len(ids) >= 10000 {
		return failure(409, "ARCHIVE_COMMAND_CATALOG_FULL", "命令证明目录已满")
	}
	r := commandRecord{Protocol: 2, ID: id, ExecutionID: execution, State: "idle", Host: s.hostIdentity, At: time.Now().UTC()}
	if job {
		r.JobID = id
	}
	if len(probes) > 0 {
		if job || len(probes) != 1 || !validProbeBinding(probes[0], id) {
			return failure(400, "ARCHIVE_PROBE_SCOPE_INVALID", "探测目标绑定无效")
		}
		r.Protocol = 3
		r.Probe = probes[0]
	}
	return dispositionWrite(s.path("command-executions", id), r)
}
func (s *Service) commandState(r commandRecord) string {
	if r.Host != s.hostIdentity {
		return "unconfirmed"
	}
	if r.State == "idle" || r.State == "drained" || s.commandConfirmationCurrent(r) {
		return "drained"
	}
	release, e := s.lock(context.Background(), "command-"+r.ID)
	if e == nil {
		release()
		return "unconfirmed"
	}
	if errors.Is(e, ErrBusy) {
		return "active"
	}
	return "unconfirmed"
}
func (s *Service) commandOverview() CommandDrainOverview {
	out := CommandDrainOverview{Protocol: 3}
	seen := map[string]bool{}
	add := func(id string) {
		if validID(id) && !seen[id] && len(out.Jobs) < 100 {
			seen[id] = true
			out.Jobs = append(out.Jobs, id)
		}
	}
	ids, e := s.records("command-executions")
	if e != nil || len(ids) > 10000 {
		out.Unconfirmed = 1
		out.Problem = "ARCHIVE_COMMAND_PROOF_UNAVAILABLE"
		return out
	}
	for _, id := range ids {
		r, err := s.readCommandRecord(id)
		if err != nil {
			out.Unconfirmed++
			out.Problem = "ARCHIVE_COMMAND_PROOF_INVALID"
			continue
		}
		switch s.commandState(r) {
		case "active":
			out.Active++
		case "unconfirmed":
			if len(out.Records) < 100 {
				item := CommandRecordAttention{RecordID: r.ID, JobID: r.JobID, Subject: "job"}
				if r.JobID == "" {
					item.Subject = "probe"
					if r.Probe != nil {
						item.TargetID = r.Probe.TargetID
						item.Name = r.Probe.Settings.Name
					}
				}
				out.Records = append(out.Records, item)
			}
			add(r.JobID)
			out.Unconfirmed++
			out.Problem = "ARCHIVE_COMMAND_DRAIN_UNCONFIRMED"
		}
	}
	jobs, err := s.records("jobs")
	if err != nil || len(jobs) > 10000 {
		out.Unconfirmed++
		out.Problem = "ARCHIVE_COMMAND_PROOF_UNAVAILABLE"
		return out
	}
	present := map[string]bool{}
	for _, id := range ids {
		present[id] = true
	}
	for _, id := range jobs {
		j, err := s.readJob(id)
		if err != nil {
			out.Unconfirmed++
			out.Problem = "ARCHIVE_COMMAND_PROOF_INVALID"
			continue
		}
		if j.CommandProtocol == 1 || j.CommandProtocol == 2 {
			r, e := s.readCommandRecord(id)
			if e != nil {
				add(j.ID)
			}
			if !present[id] || e == nil && (r.ExecutionID != j.ExecutionID || r.JobID != j.ID) {
				add(j.ID)
				out.Unconfirmed++
				out.Problem = "ARCHIVE_COMMAND_DRAIN_UNCONFIRMED"
			}
		} else if !j.Imported && !j.Recovered && (interruptionPending(j) || recordedPending(j) || j.Status == "retained" || j.Status == "failed" && (j.SnapshotID != "" || j.Target != nil && j.Target.Driver != "s3")) {
			out.Unconfirmed++
			out.Problem = "ARCHIVE_COMMAND_LEGACY_UNPROVEN"
			add(j.ID)
		}
	}
	return out
}
func (s *Service) requireCommandsDrained(allowActive bool) error {
	v := s.commandOverview()
	if v.Unconfirmed > 0 {
		return failure(409, "ARCHIVE_COMMAND_DRAIN_UNCONFIRMED", "本机外部命令排空尚未确认；保留原任务与内容，请人工核对执行归属")
	}
	if !allowActive && v.Active > 0 {
		return failure(409, "ARCHIVE_COMMANDS_ACTIVE", "本机外部命令仍在运行，不能处置或回收")
	}
	return nil
}
func (s *Service) jobCommandState(j jobRecord) string {
	if j.CommandProtocol != 1 && j.CommandProtocol != 2 {
		return "legacy_unproven"
	}
	r, e := s.readCommandRecord(j.ID)
	if e != nil || r.JobID != j.ID || r.ExecutionID != j.ExecutionID {
		return "unconfirmed"
	}
	return s.commandState(r)
}
func (s *Service) requireJobCommandsDrained(j jobRecord) error {
	if s.jobCommandState(j) != "drained" {
		return failure(409, "ARCHIVE_COMMAND_DRAIN_UNCONFIRMED", "原任务缺少可确认的本机命令排空证明；不能仅凭策略锁放行")
	}
	return s.requireCommandsDrained(false)
}

// runArchiveCommand 的组 leader 在发出最后信号之前不被 Wait 回收，PID/PGID 不会被重用。
// 仅固定 pg_dump/OpenSSH/rsync 的本机进程组适用，不证明异地主机进程已结束。
func (s *Service) runArchiveCommand(ctx context.Context, cmd *exec.Cmd, id, kind string) error {
	if kind != "pg_dump" && kind != "ssh" && kind != "rsync" {
		return failure(400, "ARCHIVE_COMMAND_KIND_INVALID", "仅支持固定归档命令")
	}
	if e := s.requireCommandsDrained(true); e != nil {
		return e
	}
	s.commandMu.Lock()
	if s.commandClosed || s.ctx.Err() != nil {
		s.commandMu.Unlock()
		return failure(503, "ARCHIVE_SERVICE_STOPPED", "归档服务已停止")
	}
	s.commandWorkers.Add(1)
	s.commandMu.Unlock()
	defer s.commandWorkers.Done()
	if e := ctx.Err(); e != nil {
		return e
	}
	release, e := s.lock(ctx, "command-"+id)
	if e != nil {
		return failure(409, "ARCHIVE_COMMANDS_ACTIVE", "原本机命令仍在执行")
	}
	defer release()
	r, e := s.readCommandRecord(id)
	if e != nil {
		return e
	}
	if s.commandStateWithoutLock(r) != "drained" {
		return failure(409, "ARCHIVE_COMMAND_DRAIN_UNCONFIRMED", "原本机命令排空尚未确认")
	}
	if r.Probe == nil {
		r.Protocol = 2
	}
	r.Identity = nil
	r.CommandID = uuid.NewString()
	r.Kind = kind
	r.State = "prepared"
	r.PID = 0
	r.PGID = 0
	r.At = time.Now().UTC()
	if e = dispositionWrite(s.path("command-executions", id), r); e != nil {
		return e
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second
	// 单独绑定 Service 取消，HTTP 调用方传入 Background 也不能在 Close 后留下命令。
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	if e = run.Err(); e != nil {
		r.State = "drained"
		_ = dispositionWrite(s.path("command-executions", id), r)
		return e
	}
	if e = cmd.Start(); e != nil {
		r.State = "drained"
		if err := dispositionWrite(s.path("command-executions", id), r); err != nil {
			return err
		}
		return e
	}
	r.State = "running"
	r.PID = cmd.Process.Pid
	r.PGID = r.PID
	identity, identityErr := captureCommandIdentity(r.PID)
	r.Identity = identity
	writeErr := dispositionWrite(s.path("command-executions", id), r)
	if identityErr != nil {
		writeErr = identityErr
	}
	exited := observeCommandExit(r.PID)
	canceled := false
	if writeErr == nil {
		select {
		case e = <-exited:
		case <-run.Done():
			canceled = true
		}
	}
	if canceled || writeErr != nil {
		_ = syscall.Kill(-r.PGID, syscall.SIGTERM)
		timer := time.NewTimer(time.Second)
		select {
		case e = <-exited:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
		}
	}
	// 同组残留可能在 leader 正常退出后继续占用管道或写入；回收 leader 前统一结束。
	_ = syscall.Kill(-r.PGID, syscall.SIGKILL)
	waitErr := cmd.Wait()
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := syscall.Kill(-r.PGID, 0)
		if errors.Is(err, syscall.ESRCH) {
			break
		}
		if time.Now().After(deadline) {
			return failure(503, "ARCHIVE_COMMAND_DRAIN_UNCONFIRMED", "本机命令组尚未完全消失")
		}
		time.Sleep(10 * time.Millisecond) // 本机退出检查，不是页面状态轮询。
	}
	if e != nil || writeErr != nil || errors.Is(waitErr, exec.ErrWaitDelay) {
		return failure(503, "ARCHIVE_COMMAND_DRAIN_UNCONFIRMED", "本机命令退出证明不完整")
	}
	r.State = "drained"
	r.At = time.Now().UTC()
	if e = dispositionWrite(s.path("command-executions", id), r); e != nil {
		return e
	}
	if canceled {
		return run.Err()
	}
	return waitErr
}
func (s *Service) commandStateWithoutLock(r commandRecord) string {
	if r.Host == s.hostIdentity && (r.State == "idle" || r.State == "drained" || s.commandConfirmationCurrent(r)) {
		return "drained"
	}
	return "unconfirmed"
}
func commandProbeID(target string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("archive-command-probe/"+target)).String()
}
func (s *Service) commandForRemote(jobID string, target Target) (string, error) {
	id := jobID
	if !validID(id) {
		return s.initializeProbeCommand(target)
	}
	_, e := s.readCommandRecord(id)
	if dto.StatusCode(e) == 404 {
		if j, err := s.readJob(jobID); err == nil && (j.CommandProtocol == 1 || j.CommandProtocol == 2) {
			return "", failure(409, "ARCHIVE_COMMAND_DRAIN_UNCONFIRMED", "原任务命令证明缺失")
		}
		e = s.initializeCommandRecord(id, uuid.NewString(), validID(jobID))
	}
	return id, e
}
