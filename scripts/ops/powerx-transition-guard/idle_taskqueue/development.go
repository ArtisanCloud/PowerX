package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// effectiveRetentionHours 保留旧临时任务七天约束；24 小时只允许显式 dev 维护。
func effectiveRetentionHours(devHours int) int {
	if devHours == 24 {
		return 24
	}
	return 7 * 24
}
func validRetentionCutoff(database string, devHours int, commit string, cutoff, now time.Time) error {
	if devHours != 0 && devHours != 24 {
		return errors.New("invalid_development_retention")
	}
	if devHours == 24 {
		decoded, err := hex.DecodeString(commit)
		if database != "powerx_dev" || err != nil || len(decoded) != 20 {
			return errors.New("development_scope_required")
		}
	} else if commit != "" {
		return errors.New("unexpected_development_commit")
	}
	if cutoff.IsZero() || now.Sub(cutoff) < time.Duration(effectiveRetentionHours(devHours))*time.Hour {
		return errors.New("retention_floor")
	}
	return nil
}
func checkDevelopmentCommit(path, expected string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return errors.New("development_runtime_unavailable")
	}
	var manifest struct {
		SourceCommit string `json:"source_commit"`
	}
	if json.Unmarshal(raw, &manifest) != nil || manifest.SourceCommit != expected {
		return errors.New("development_runtime_changed_stop_before_next_batch")
	}
	return nil
}

// maximumRetentionBatch 只允许显式 dev 维护扩大单批，原七天任务保持 1000 上限。
func maximumRetentionBatch(devHours int) int {
	if devHours == 24 {
		return 5000
	}
	return 1000
}

// developmentScopeSQL 的 receipt 是 Core“发布时零订阅者”的确认 ID，不是业务送达回执。
// 只扩展已分类、单次尝试、终态和发布时间一致的明确 unrouted；原七天任务保持不变。
func developmentScopeSQL() string {
	old := ` AND e.attempts=0 AND e.publish_time IS NULL AND COALESCE(e.delivery_receipt,'')=''
 AND ((e.status='pending' AND e.terminal_at IS NULL)
      OR (e.status='unrouted' AND e.terminal_at<$3))`
	updated := ` AND (
 (e.attempts=0 AND e.publish_time IS NULL AND COALESCE(e.delivery_receipt,'')=''
  AND ((e.status='pending' AND e.terminal_at IS NULL) OR (e.status='unrouted' AND e.terminal_at<$3)))
 OR (e.record_class='capability_invocation_notification' AND e.status='unrouted'
  AND e.attempts=1 AND e.publish_time=e.terminal_at AND e.terminal_at<$3
  AND e.delivery_receipt ~ '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
  AND COALESCE(e.last_error,'')=''))`
	if strings.Count(scopeSQL, old) != 1 {
		panic("development_scope_contract_changed")
	}
	return strings.Replace(scopeSQL, old, updated, 1)
}

func validDevelopmentScope(scope string) bool {
	b, e := hex.DecodeString(scope)
	return e == nil && len(b) == 32
}

// guardDevelopmentExecution 复用 Core 的执行域共享事务锁；全局保护一旦接管即交回 Core。
func guardDevelopmentExecution(ctx context.Context, tx pgx.Tx, scope string) error {
	if !validDevelopmentScope(scope) {
		return errors.New("invalid_development_control_scope")
	}
	sum := sha256.Sum256([]byte("powerx.storage.configuration/v1\x00" + scope + "\x00execution-control"))
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared($1)", int64(binary.BigEndian.Uint64(sum[:8]))); err != nil {
		return err
	}
	var n int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM public.ops_storage_control_configurations WHERE tenant_id=0 AND scope_id=$1 AND domain='execution-control'", scope).Scan(&n); err != nil {
		return err
	}
	if n != 0 {
		return errors.New("core_execution_control_adopted_stop_temporary_maintenance")
	}
	return nil
}
