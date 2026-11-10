package archive_ops

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

type RemoteGCResult struct {
	ID                string         `json:"id"`
	PolicyID          string         `json:"policy_id"`
	PolicyRevision    int64          `json:"policy_revision"`
	State             string         `json:"state"`
	CreatedAt         time.Time      `json:"created_at"`
	ExpiresAt         time.Time      `json:"expires_at"`
	CandidateVersions int            `json:"candidate_versions"`
	CandidateBytes    int64          `json:"candidate_bytes"`
	DeletedVersions   int            `json:"deleted_versions"`
	ConfirmedBytes    int64          `json:"confirmed_bytes"`
	Protected         map[string]int `json:"protected"`
	Problem           string         `json:"problem,omitempty"`
}
type remoteGCPlan struct {
	RemoteGCResult
	Target   Target                `json:"target"`
	Jobs     []string              `json:"jobs"`
	Versions []minio.ObjectVersion `json:"versions"`
}

func versionKey(key, version string) string { return key + "\x00" + version }
func (s *Service) remoteGCRuns() ([]RemoteGCResult, error) {
	ids, err := s.records("remote-gc")
	if err != nil {
		return nil, err
	}
	out := []RemoteGCResult{}
	for _, id := range ids {
		var p remoteGCPlan
		if err = readJSON(s.path("remote-gc", id), &p); err != nil {
			return nil, err
		}
		if p.State == "preview" && time.Now().After(p.ExpiresAt) {
			continue
		}
		out = append(out, p.RemoteGCResult)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	visible := []RemoteGCResult{}
	terminal := 0
	for _, run := range out {
		if run.State == "queued" || run.State == "running" || run.State == "unconfirmed" {
			visible = append(visible, run)
		} else if terminal < 20 {
			visible = append(visible, run)
			terminal++
		}
	}
	return visible, nil
}
func (s *Service) buildRemoteGC(ctx context.Context, p Policy, t Target, ignore ...string) (remoteGCPlan, error) {
	if e := s.requireCommandsDrained(false); e != nil {
		return remoteGCPlan{}, e
	}
	out := remoteGCPlan{RemoteGCResult: RemoteGCResult{ID: uuid.NewString(), PolicyID: p.ID, PolicyRevision: p.Revision, State: "preview", CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(5 * time.Minute), Protected: map[string]int{}}, Target: t, Jobs: []string{}, Versions: []minio.ObjectVersion{}}
	idsRuns, e := s.records("remote-gc")
	if e != nil {
		return out, e
	}
	unresolved := 0
	for _, id := range idsRuns {
		if len(ignore) > 0 && id == ignore[0] {
			continue
		}
		var old remoteGCPlan
		if readJSON(s.path("remote-gc", id), &old) != nil {
			return out, errors.New("清理运行记录不可读取")
		}
		if old.State == "running" || old.State == "queued" || old.State == "unconfirmed" {
			unresolved++
			if old.PolicyID == p.ID {
				return out, errors.New("该策略有未确认清理，请先核对具体版本")
			}
		}
	}
	if unresolved >= 100 {
		return out, errors.New("未确认清理记录达到上限，请先核对")
	}
	if p.RemoteRetentionDays < 1 || p.RemoteRetentionDays > 3650 {
		return out, errors.New("归档期限无效，停止清理")
	}
	if t.Driver != "s3" || !s.targetVerified(t) {
		return out, failure(422, "ARCHIVE_TARGET_UNVERIFIED", "请先验证当前 S3 归档目标")
	}
	ids, err := s.records("jobs")
	if err != nil {
		return out, err
	}
	jobs := map[string]jobRecord{}
	latest := time.Time{}
	for _, id := range ids {
		j, e := s.readJob(id)
		if e != nil {
			return out, e
		}
		if j.RemoteState == "expired" {
			continue
		}
		if j.PolicyID == p.ID && j.Target != nil && j.Target.ID == t.ID {
			if !sameArchiveStorage(*j.Target, t) {
				out.Protected["historical_target_configuration"]++
				continue
			}
			jobs[id] = j
			if j.CreatedAt.After(latest) {
				latest = j.CreatedAt
			}
		}
	}
	expired := map[string]bool{}
	keep := map[string]bool{}
	for id, j := range jobs {
		reason := ""
		now := time.Now()
		if s.opts.RetentionNow != nil {
			now = s.opts.RetentionNow()
		}
		switch {
		case j.Protected:
			reason = "explicit_protection"
		case j.Imported || j.Recovered:
			reason = "legacy_migration_protected"
		case j.CreatedAt.Equal(latest):
			reason = "latest_archive"
		case j.Status != "success" && j.Status != "pruned":
			reason = "incomplete_or_pending"
		case j.RemoteState != "success":
			reason = "remote_not_verified"
		case now.Sub(j.CreatedAt) < time.Duration(p.RemoteRetentionDays)*24*time.Hour:
			reason = "within_retention"
		}
		if reason != "" {
			out.Protected[reason]++
			for _, f := range j.Files {
				if f.ArchiveKey != "" {
					keep[versionKey(f.ArchiveKey, f.ArchiveVersion)] = true
				}
			}
			continue
		}
		expired[id] = true
		out.Jobs = append(out.Jobs, id)
	}
	c, err := s.s3Client(t)
	if err != nil {
		return out, err
	}
	ns := s.s3Namespace(t, p.ID) + "/"
	versions, err := listS3Versions(ctx, c, t, ns)
	if err != nil {
		return out, err
	}
	remove := map[string]minio.ObjectVersion{}
	for _, v := range versions {
		if !strings.HasPrefix(v.Key, ns+"manifests/") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(v.Key, ns+"manifests/"), ".json")
		if !validID(id) {
			return out, errors.New("发现未知远端清单，停止清理")
		}
		if _, ok := jobs[id]; !ok {
			return out, errors.New("远端清单尚未纳入持久目录，停止清理")
		}
		if v.DeleteMarker {
			if expired[id] {
				remove[versionKey(v.Key, v.VersionID)] = v
			}
			continue
		}
		if v.Size < 1 || v.Size > 4<<20 {
			return out, errors.New("远端清单大小无效，停止清理")
		}
		f, e := c.GetObject(ctx, t.Bucket, v.Key, minio.GetObjectOptions{VersionID: v.VersionID})
		if e != nil {
			return out, errors.New("远端引用清单不可读取，停止清理")
		}
		st, e := f.Stat()
		raw, readErr := io.ReadAll(io.LimitReader(f, (4<<20)+1))
		f.Close()
		var m Job
		if e != nil || readErr != nil || st.VersionID != v.VersionID || int64(len(raw)) != v.Size || json.Unmarshal(raw, &m) != nil || m.ID != id || m.PolicyID != p.ID || len(m.Files) == 0 {
			return out, errors.New("远端引用清单身份不符，停止清理")
		}
		if len(m.Files) != len(jobs[id].Files) {
			return out, errors.New("远端清单与本地持久目录不一致，停止清理")
		}
		for i, f := range m.Files {
			want := jobs[id].Files[i]
			if f.Path != want.Path || f.Size != want.Size || f.SHA256 != want.SHA256 || f.ArchiveKey != want.ArchiveKey || f.ArchiveVersion != want.ArchiveVersion {
				return out, errors.New("远端引用与本地持久目录不一致，停止清理")
			}
		}
		for _, entry := range m.Files {
			if !shaPattern.MatchString(entry.SHA256) || entry.ArchiveVersion == "" || entry.ArchiveVersion == "null" || entry.ArchiveKey != ns+"objects/"+entry.SHA256[:2]+"/"+entry.SHA256 {
				return out, errors.New("远端对象引用越界，停止清理")
			}
			if !expired[id] {
				keep[versionKey(entry.ArchiveKey, entry.ArchiveVersion)] = true
			}
		}
		if expired[id] {
			remove[versionKey(v.Key, v.VersionID)] = v
		}
	}
	refs := map[string]bool{}
	for id, j := range jobs {
		if expired[id] {
			for _, f := range j.Files {
				refs[versionKey(f.ArchiveKey, f.ArchiveVersion)] = true
			}
		}
	}
	for _, v := range versions {
		key := versionKey(v.Key, v.VersionID)
		if refs[key] && !keep[key] {
			remove[key] = v
		}
	}
	for _, v := range remove {
		out.Versions = append(out.Versions, v)
		out.CandidateBytes += v.Size
	}
	sort.Strings(out.Jobs)
	sort.Slice(out.Versions, func(i, j int) bool {
		return versionKey(out.Versions[i].Key, out.Versions[i].VersionID) < versionKey(out.Versions[j].Key, out.Versions[j].VersionID)
	})
	out.CandidateVersions = len(out.Versions)
	if len(out.Versions) > 5000 {
		return out, errors.New("清理批次版本超过 5000，停止执行；请拆分策略范围")
	}
	return out, nil
}
func (s *Service) PreviewRemoteGC(ctx context.Context, policyID string) (RemoteGCResult, error) {
	if err := s.check(ctx); err != nil {
		return RemoteGCResult{}, err
	}
	p, err := s.policy(ctx, policyID)
	if err != nil {
		return RemoteGCResult{}, err
	}
	t, err := s.targetContext(ctx, p.TargetID)
	if err != nil {
		return RemoteGCResult{}, err
	}
	unlock, err := s.lock(ctx, p.ID)
	if err != nil {
		return RemoteGCResult{}, ErrBusy
	}
	defer unlock()
	plan, err := s.buildRemoteGC(ctx, p, t)
	if err != nil {
		return plan.RemoteGCResult, err
	}
	if err = writeJSON(s.path("remote-gc", plan.ID), plan); err != nil {
		return plan.RemoteGCResult, err
	}
	s.pruneRemoteGCPreviews()
	return plan.RemoteGCResult, nil
}
func (s *Service) pruneRemoteGCPreviews() { s.pruneRemoteGCMetadata() }
func (s *Service) StartRemoteGC(ctx context.Context, id string) (RemoteGCResult, error) {
	if err := s.check(ctx); err != nil {
		return RemoteGCResult{}, err
	}
	if !validID(id) {
		return RemoteGCResult{}, failure(400, "ARCHIVE_ID_INVALID", "清理预览编号无效")
	}
	unlock, err := s.lock(ctx, "gc-"+id)
	if err != nil {
		return RemoteGCResult{}, ErrBusy
	}
	var plan remoteGCPlan
	if err = readJSON(s.path("remote-gc", id), &plan); err != nil {
		unlock()
		return RemoteGCResult{}, err
	}
	if plan.State != "preview" {
		unlock()
		return plan.RemoteGCResult, nil
	}
	if time.Now().After(plan.ExpiresAt) {
		unlock()
		return plan.RemoteGCResult, failure(409, "ARCHIVE_PREVIEW_EXPIRED", "清理预览已过期，请重新预览")
	}
	if s.policyControlRequired() {
		p, e := s.policy(ctx, plan.PolicyID)
		if e != nil || !p.RemoteCleanupEnabled || p.Revision != plan.PolicyRevision {
			unlock()
			return plan.RemoteGCResult, failure(409, "ARCHIVE_POLICY_DISABLED", "策略已停用或版本变化，请重新预览")
		}
	}
	plan.State = "queued"
	if err = writeJSON(s.path("remote-gc", id), plan); err != nil {
		unlock()
		return plan.RemoteGCResult, err
	}
	s.mu.Lock()
	if s.ctx.Err() != nil {
		s.mu.Unlock()
		unlock()
		return plan.RemoteGCResult, errors.New("归档服务已停止")
	}
	s.workers.Add(1)
	s.mu.Unlock()
	// 先冻结响应；worker 会立即推进 plan，不能与 HTTP 序列化共享可变状态。
	response := plan.RemoteGCResult
	response.Protected = make(map[string]int, len(plan.Protected))
	for key, count := range plan.Protected {
		response.Protected[key] = count
	}
	go func() {
		defer s.workers.Done()
		defer unlock()
		run, cancel := context.WithTimeout(s.ctx, 5*time.Minute)
		defer cancel()
		s.executeRemoteGC(run, &plan)
	}()
	return response, nil
}
func (s *Service) executeRemoteGC(ctx context.Context, plan *remoteGCPlan) {
	if !s.policyControlRequired() {
		s.executeRemoteGCApproved(ctx, plan)
		return
	}
	p, e := s.policy(ctx, plan.PolicyID)
	if e != nil || p.Revision != plan.PolicyRevision || !p.RemoteCleanupEnabled {
		plan.State = "blocked"
		plan.Problem = "策略已停用或版本变化"
		_ = writeJSON(s.path("remote-gc", plan.ID), plan)
		return
	}
	e = s.guardPolicyAction(ctx, p, "remote-cleanup", func(run context.Context) error { s.executeRemoteGCApproved(run, plan); return run.Err() })
	if e != nil {
		if plan.DeletedVersions > 0 {
			plan.State = "unconfirmed"
		} else {
			plan.State = "blocked"
		}
		plan.Problem = "策略、目标或执行条件发生变化，已停止本批操作"
		_ = writeJSON(s.path("remote-gc", plan.ID), plan)
	}
}
func (s *Service) executeRemoteGCApproved(ctx context.Context, plan *remoteGCPlan) {
	finish := func(state, problem string) {
		plan.State = state
		plan.Problem = problem
		_ = writeJSON(s.path("remote-gc", plan.ID), plan)
	}
	unlock, err := s.lock(ctx, plan.PolicyID)
	if err != nil {
		finish("blocked", "来源正在归档、下载或清理，请重新预览")
		return
	}
	defer unlock()
	settings := func() {}
	if !s.policyControlRequired() {
		settings, err = s.lock(ctx, "settings")
	}
	if err != nil {
		finish("blocked", "策略正在修改，请重新预览")
		return
	}
	defer settings()
	p, err := s.policy(ctx, plan.PolicyID)
	if err != nil || p.Revision != plan.PolicyRevision || s.policyControlRequired() && !p.RemoteCleanupEnabled {
		finish("blocked", "策略版本已变化")
		return
	}
	t, err := s.targetContext(ctx, p.TargetID)
	if err != nil || t.Revision != plan.Target.Revision || t.VerifiedFingerprint != plan.Target.VerifiedFingerprint {
		finish("blocked", "归档目标已变化")
		return
	}
	current, err := s.buildRemoteGC(ctx, p, t, plan.ID)
	if err != nil {
		finish("blocked", "远端引用无法重新核对")
		return
	}
	want, _ := json.Marshal(plan.Versions)
	got, _ := json.Marshal(current.Versions)
	wantJobs, _ := json.Marshal(plan.Jobs)
	gotJobs, _ := json.Marshal(current.Jobs)
	if string(want) != string(got) || string(wantJobs) != string(gotJobs) {
		finish("blocked", "归档引用或版本在预览后变化，请重新预览")
		return
	}
	c, err := s.s3Client(t)
	if err != nil {
		finish("blocked", "归档目标不可用")
		return
	}
	finish("running", "")
	for _, id := range plan.Jobs {
		j, e := s.readJob(id)
		if e != nil {
			finish("unconfirmed", "清理意图不能完整记录")
			return
		}
		j.Status = "expiry_pending"
		j.RemoteState = "expiry_pending"
		if s.saveJob(&j) != nil {
			finish("unconfirmed", "清理意图不能完整记录")
			return
		}
	}
	for _, v := range plan.Versions {
		if ctx.Err() != nil {
			finish("unconfirmed", "清理中断，需核对具体版本结果")
			return
		}
		err = s.guardPolicyAction(ctx, p, "remote-cleanup", func(run context.Context) error {
			return c.RemoveObject(run, t.Bucket, v.Key, minio.RemoveObjectOptions{VersionID: v.VersionID})
		})
		if err != nil {
			finish("unconfirmed", "版本删除结果未确认，停止后续删除")
			return
		}
		f, e := c.GetObject(ctx, t.Bucket, v.Key, minio.GetObjectOptions{VersionID: v.VersionID})
		if e == nil {
			f.Close()
			finish("unconfirmed", "删除后对象版本仍可读取")
			return
		}
		if !missingS3Config(e, "NoSuchVersion", "NoSuchKey", "NotFound") {
			finish("unconfirmed", "删除后无法确认对象不存在")
			return
		}
		plan.DeletedVersions++
		plan.ConfirmedBytes += v.Size
		if writeJSON(s.path("remote-gc", plan.ID), plan) != nil {
			return
		}
	}
	for _, id := range plan.Jobs {
		j, e := s.readJob(id)
		if e != nil {
			finish("unconfirmed", "对象已处理，目录更新待核对")
			return
		}
		j.Status = "expired"
		j.RemoteState = "expired"
		if s.saveJob(&j) != nil {
			finish("unconfirmed", "对象已处理，目录更新待核对")
			return
		}
	}
	finish("completed", "")
	s.auditAction(ctx, "remote_expiry", plan.PolicyID, plan.PolicyRevision)
}

// tickRemoteGC 不依赖新归档产生；用户单独开启后，每小时检查一次到期对象。
func (s *Service) tickRemoteGC(ctx context.Context, p Policy) error {
	if !p.RemoteCleanupEnabled {
		return nil
	}
	marker := s.path("remote-gc", p.ID+".latest")
	var previous struct {
		At time.Time `json:"at"`
	}
	if err := readJSON(marker, &previous); err == nil && time.Since(previous.At) < time.Hour {
		return nil
	}
	t, err := s.targetContext(ctx, p.TargetID)
	if err != nil {
		return err
	}
	plan, err := s.buildRemoteGC(ctx, p, t)
	if err != nil {
		return err
	}
	if err = writeJSON(s.path("remote-gc", plan.ID), plan); err != nil {
		return err
	}
	if err = writeJSON(marker, struct {
		At time.Time `json:"at"`
	}{time.Now().UTC()}); err != nil {
		return err
	}
	s.executeRemoteGC(ctx, &plan)
	if plan.State != "completed" {
		return errors.New("归档到期任务受保护或结果待核对，请查看归档清理记录")
	}
	s.pruneRemoteGCPreviews()
	return nil
}

// sameArchiveStorage 只判断远端命名空间；改名或凭据轮换不改变存储位置。
func sameArchiveStorage(a, b Target) bool {
	return a.Driver == b.Driver && a.Endpoint == b.Endpoint && a.Bucket == b.Bucket && a.Prefix == b.Prefix && a.Host == b.Host && a.Port == b.Port && a.User == b.User && a.Directory == b.Directory
}
