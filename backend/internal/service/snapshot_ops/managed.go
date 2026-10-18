package snapshot_ops

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
)

// RecoveryTarget 由本机私有 profile 预先登记，HTTP 只能选择 ID。
type RecoveryTarget struct {
	ID              string `json:"id"`
	Label           string `json:"label"`
	Environment     string `json:"environment"`
	Mode            string `json:"mode"` // isolated 或 formal
	AllowOverwrite  bool   `json:"allow_overwrite,omitempty"`
	AllowOnlineData bool   `json:"allow_online_data,omitempty"`
}

// ManagedJob 独立于业务库保存，切换确认绑定具体候选与当前部署代次。
type ManagedJob struct {
	RestoreMode          string          `json:"restore_mode,omitempty"`
	ValidationURL        string          `json:"validation_url,omitempty"`
	ID                   string          `json:"id"`
	Kind                 string          `json:"kind"`
	SnapshotID           string          `json:"snapshot_id"`
	TargetID             string          `json:"target_id,omitempty"`
	CandidateID          string          `json:"candidate_id,omitempty"`
	PreviousID           string          `json:"previous_id,omitempty"`
	RollbackSnapshotID   string          `json:"rollback_snapshot_id,omitempty"`
	ExpectedGeneration   uint64          `json:"expected_generation,omitempty"`
	Status               string          `json:"status"`
	Phase                string          `json:"phase"`
	BusinessVerified     bool            `json:"business_verified"`
	WritesMayHaveResumed bool            `json:"writes_may_have_resumed"`
	Error                string          `json:"error,omitempty"`
	Checks               []RecoveryCheck `json:"checks"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
	Operator             string          `json:"operator"`
	TraceID              string          `json:"trace_id,omitempty"`
	RequestHash          string          `json:"request_hash"`
}
type RecoveryCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// CoordinatorStore 是 Core 与独立 worker 的本机持久队列；目录权限就是其服务权限。
type CoordinatorStore struct {
	Directory string
	Targets   []RecoveryTarget
	// 同进程维护模式使用正常 Core 的地址；这些回调不进入持久化文件。
	RecoveryEntry  string
	RestoreProblem string
	PrepareSession func(http.ResponseWriter, *http.Request, string) error
}

// Online 只读取协调器心跳，不把“目录存在”误报为执行器在线。
func (c *CoordinatorStore) Online() bool {
	var heartbeat struct {
		At time.Time `json:"at"`
	}
	return readJSON(filepath.Join(c.Directory, "heartbeat.json"), &heartbeat) == nil && time.Since(heartbeat.At) >= 0 && time.Since(heartbeat.At) < 10*time.Second
}

// ProgressToken 是只读、单任务、有期限的进度票据；不会授予任何恢复或切换权限。
func (c *CoordinatorStore) ProgressToken(id string) (string, error) {
	if !validID(id) {
		return "", errors.New("invalid job id")
	}
	key, e := c.progressKey()
	if e != nil {
		return "", e
	}
	expires := strconv.FormatInt(time.Now().Add(12*time.Hour).Unix(), 10)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + ":" + expires))
	return expires + "." + hex.EncodeToString(mac.Sum(nil)), nil
}
func (c *CoordinatorStore) VerifyProgressToken(id, token string) bool {
	if !validID(id) {
		return false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}
	expires, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil || time.Now().Unix() > expires || expires > time.Now().Add(12*time.Hour+time.Minute).Unix() {
		return false
	}
	key, e := c.progressKey()
	if e != nil {
		return false
	}
	signature, e := hex.DecodeString(parts[1])
	if e != nil {
		return false
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + ":" + parts[0]))
	return hmac.Equal(signature, mac.Sum(nil))
}
func (c *CoordinatorStore) progressKey() ([]byte, error) {
	unlock, e := lockFile(filepath.Join(c.Directory, "locks", "status-key.lock"))
	if e != nil {
		return nil, e
	}
	defer unlock()
	file := filepath.Join(c.Directory, "status.key")
	key, e := os.ReadFile(file)
	if errors.Is(e, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, e = rand.Read(key); e == nil {
			e = atomicFile(file, key)
		}
	}
	if e != nil || len(key) != 32 {
		return nil, errors.New("progress key unavailable")
	}
	info, e := os.Lstat(file)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("progress key is not private")
	}
	return key, nil
}

func NewCoordinatorStore(directory string, targets []RecoveryTarget) (*CoordinatorStore, error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("coordinator directory must be absolute")
	}
	for _, t := range targets {
		if !regexp.MustCompile(`^[a-z][a-z0-9_-]{0,47}$`).MatchString(t.ID) || (t.Mode != "isolated" && t.Mode != "formal") {
			return nil, errors.New("invalid recovery target")
		}
		if t.AllowOnlineData && t.Environment != "dev" {
			return nil, errors.New("在线数据包演练仅允许显式登记的 dev 目标")
		}
	}
	for _, kind := range []string{"jobs", "locks"} {
		if e := os.MkdirAll(filepath.Join(directory, kind), 0700); e != nil {
			return nil, e
		}
	}
	return &CoordinatorStore{Directory: directory, Targets: targets}, nil
}
func (c *CoordinatorStore) jobFile(id string) string {
	return filepath.Join(c.Directory, "jobs", id+".json")
}
func (c *CoordinatorStore) Save(j ManagedJob) error {
	if !validID(j.ID) {
		return errors.New("invalid job id")
	}
	j.UpdatedAt = time.Now().UTC()
	return atomicJSON(c.jobFile(j.ID), j)
}
func (c *CoordinatorStore) Get(id string) (ManagedJob, error) {
	var j ManagedJob
	if !validID(id) {
		return j, errors.New("invalid job id")
	}
	e := readJSON(c.jobFile(id), &j)
	if e == nil && j.ID != id {
		e = errors.New("job identity mismatch")
	}
	return j, e
}
func (c *CoordinatorStore) List() ([]ManagedJob, error) {
	entries, e := os.ReadDir(filepath.Join(c.Directory, "jobs"))
	if e != nil {
		return nil, e
	}
	out := []ManagedJob{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		id := entry.Name()
		if filepath.Ext(id) != ".json" {
			continue
		}
		j, e := c.Get(id[:len(id)-5])
		if e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, nil
}
func (c *CoordinatorStore) Lock() (func(), error) {
	return lockFile(filepath.Join(c.Directory, "locks", "worker.lock"))
}
func (c *CoordinatorStore) EnqueueCapture(ctx context.Context, id string) error {
	_, e := c.enqueue(ctx, "capture", id, "", id, 0, "")
	return e
}
func (c *CoordinatorStore) EnqueueRestore(ctx context.Context, snapshot, target, key string) (ManagedJob, error) {
	return c.EnqueueRestoreMode(ctx, snapshot, target, key, "new_environment")
}

// EnqueueInstanceRestore replaces the one registered installation. There is no
// browser-selected DSN, directory, environment, or separately authorized cutover.
func (c *CoordinatorStore) EnqueueInstanceRestore(ctx context.Context, snapshot, key string, generation uint64) (ManagedJob, error) {
	if e := root(ctx); e != nil {
		return ManagedJob{}, e
	}
	target := ""
	for _, t := range c.Targets {
		if !t.AllowOverwrite {
			continue
		}
		if target != "" {
			return ManagedJob{}, failure(409, "RECOVERY_TARGET_AMBIGUOUS", "只能登记一个当前实例恢复目标")
		}
		target = t.ID
	}
	if target == "" {
		return ManagedJob{}, failure(409, "RECOVERY_INSTALLATION_REQUIRED", "当前实例尚未接入原地恢复，请先完成部署配置")
	}
	return c.enqueue(ctx, "restore", snapshot, target, key, generation, "", "replace_instance")
}
func (c *CoordinatorStore) EnqueueRestoreMode(ctx context.Context, snapshot, target, key, mode string) (ManagedJob, error) {
	if e := root(ctx); e != nil {
		return ManagedJob{}, e
	}
	found := false
	if mode == "" {
		mode = "new_environment"
	}
	if mode != "new_environment" && mode != "overwrite_current" {
		return ManagedJob{}, failure(400, "RECOVERY_MODE_INVALID", "请选择新建环境或本地原地覆盖")
	}
	for _, t := range c.Targets {
		if t.ID == target {
			found = true
			if mode == "overwrite_current" && (!t.AllowOverwrite || t.Environment != "dev" || t.Mode != "formal") {
				return ManagedJob{}, failure(403, "RECOVERY_OVERWRITE_FORBIDDEN", "原地覆盖仅允许显式登记的本地开发目标")
			}
		}
	}
	if !found {
		return ManagedJob{}, failure(400, "RECOVERY_TARGET_INVALID", "请选择已登记的目标环境")
	}
	return c.enqueue(ctx, "prepare", snapshot, target, key, 0, "", mode)
}
func (c *CoordinatorStore) EnqueueCutover(ctx context.Context, candidate, key string, generation uint64) (ManagedJob, error) {
	if e := root(ctx); e != nil {
		return ManagedJob{}, e
	}
	j, e := c.Get(candidate)
	if e != nil {
		return j, e
	}
	formal := false
	for _, t := range c.Targets {
		if t.ID == j.TargetID && t.Mode == "formal" {
			formal = true
		}
	}
	if !formal || j.Kind != "prepare" || j.Status != "verified" || !j.BusinessVerified {
		return ManagedJob{}, failure(409, "RECOVERY_NOT_VERIFIED", "只有正式候选通过业务验证后才能切换")
	}
	kind := "cutover"
	if j.RestoreMode == "overwrite_current" {
		kind = "overwrite"
	}
	return c.enqueue(ctx, kind, j.SnapshotID, j.TargetID, key, generation, j.ID, j.RestoreMode)
}

// EnqueueResume 只继续当前候选的现有数据，不执行可能丢失新写入的旧库覆盖。
func (c *CoordinatorStore) EnqueueResume(ctx context.Context, jobID, key string, generation uint64) (ManagedJob, error) {
	if e := root(ctx); e != nil {
		return ManagedJob{}, e
	}
	previous, e := c.Get(jobID)
	if e != nil {
		return previous, e
	}
	if (previous.Kind != "cutover" && previous.Kind != "resume" && previous.Kind != "restore") || previous.Status != "attention_required" || !validID(previous.CandidateID) {
		return ManagedJob{}, failure(409, "RECOVERY_NOT_HELD", "该任务不是需要继续的候选维护现场")
	}
	return c.enqueue(ctx, "resume", previous.SnapshotID, previous.TargetID, key, generation, previous.CandidateID)
}

func (c *CoordinatorStore) enqueue(ctx context.Context, kind, snapshot, target, key string, generation uint64, candidate string, modes ...string) (ManagedJob, error) {
	mode := ""
	if len(modes) > 0 {
		mode = modes[0]
	}
	if !validID(snapshot) || !validID(key) {
		return ManagedJob{}, failure(400, "RECOVERY_ID_INVALID", "快照和幂等键必须为 UUID")
	}
	unlock, e := lockFile(filepath.Join(c.Directory, "locks", "queue.lock"))
	if e != nil {
		return ManagedJob{}, e
	}
	defer unlock()
	hash := digest([]byte(kind + ":" + key))
	jobs, e := c.List()
	if e != nil {
		return ManagedJob{}, e
	}
	for _, j := range jobs {
		if j.RequestHash == hash {
			priorMode := j.RestoreMode
			if kind == "prepare" && priorMode == "" {
				priorMode = "new_environment"
			}
			if j.SnapshotID != snapshot || j.TargetID != target || j.ExpectedGeneration != generation || (kind != "restore" && j.CandidateID != candidate) || priorMode != mode {
				return j, failure(409, "RECOVERY_IDEMPOTENCY_CONFLICT", "幂等键请求内容不一致")
			}
			return j, nil
		}
	}
	if kind == "restore" {
		state, e := c.EnvironmentState()
		if e != nil {
			return ManagedJob{}, e
		}
		if state["maintenance"] == true || generation == 0 || state["generation"] != generation {
			return ManagedJob{}, failure(409, "RECOVERY_INSTANCE_CHANGED", "当前实例正在维护或已变化，请刷新后重新确认恢复")
		}
		for _, j := range jobs {
			if j.Status == "queued" || j.Status == "running" || j.Status == "attention_required" {
				return ManagedJob{}, failure(409, "RECOVERY_BUSY", "已有任务待完成或需处理，不能同时恢复当前实例")
			}
		}
	}
	if kind == "resume" {
		state, e := c.EnvironmentState()
		if e != nil {
			return ManagedJob{}, e
		}
		if state["maintenance"] != true || generation == 0 || state["generation"] != generation {
			return ManagedJob{}, failure(409, "RECOVERY_INSTANCE_CHANGED", "维护现场已变化，请刷新后重新确认")
		}
		for _, j := range jobs {
			if j.Status == "queued" || j.Status == "running" {
				return ManagedJob{}, failure(409, "RECOVERY_BUSY", "已有恢复任务执行中，请勿重复提交")
			}
		}
	}
	now := time.Now().UTC()
	j := ManagedJob{RestoreMode: mode, ID: uuid.NewString(), Kind: kind, SnapshotID: snapshot, TargetID: target, CandidateID: candidate, ExpectedGeneration: generation, Status: "queued", Phase: "queued", Checks: []RecoveryCheck{}, CreatedAt: now, UpdatedAt: now, Operator: operator(ctx), TraceID: reqctx.GetTraceID(ctx), RequestHash: hash}
	return j, c.Save(j)
}

// WriteCoordinatorJSON / ReadCoordinatorJSON 只供可信本机协调器使用，沿用 fsync 原子写。
func WriteCoordinatorJSON(file string, value any) error { return atomicJSON(file, value) }
func ReadCoordinatorJSON(file string, value any) error {
	if _, e := os.Lstat(file); e != nil {
		return e
	}
	return readJSON(file, value)
}
