package main

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func guardReferences(ctx context.Context, tx pgx.Tx) error {
	var n int
	if err := tx.QueryRow(ctx, `SELECT
	 (SELECT count(*) FROM pg_constraint WHERE contype='f'
	  AND (confrelid IN ($1::regclass,$2::regclass) OR conrelid IN ($1::regclass,$2::regclass)))
	 +(SELECT count(*) FROM pg_trigger WHERE NOT tgisinternal AND tgrelid IN ($1::regclass,$2::regclass))`, traces, events).Scan(&n); err != nil {
		return err
	}
	if n != 0 {
		return errors.New("references_or_triggers_changed")
	}
	return nil
}

// 固定只读消费合同与唯一通知证明。未知来源、真实消息、重试／失败、
// 已投递通知及最近七天均保留；不把过期的未投递通知改成成功。
const scopeSQL = ` FROM ` + traces + ` t JOIN ` + events + ` e ON e.uuid=t.event_publication_id
 WHERE t.id>$1 AND t.id<=$2 AND t.created_at<$3 AND t.updated_at<$3
 AND e.created_at<$3 AND e.updated_at<$3 AND t.deleted_at IS NULL AND e.deleted_at IS NULL
 AND t.capability_id='com.corex.runtime.taskqueue.service_consume'
 AND t.plugin_id='corex.platform' AND t.caller_subject LIKE 'sts:com.powerx.plugins.ai-craft:%'
 AND t.status='completed' AND t.protocol_used='core_internal' AND t.fallback_used=false
 AND COALESCE(t.error_summary,'')='' AND t.event_published=false
 AND t.record_class IN ('capability_invocation_audit','legacy_unclassified')
 AND (t.completed_at<$3 OR (t.record_class='legacy_unclassified' AND t.completed_at IS NULL))
 AND t.request_payload#>>'{body,contract_version}'='taskqueue.v2'
 AND t.request_payload#>>'{body,operation}'='dequeue'
 AND t.request_payload#>>'{body,tenant_key}'=t.tenant_uuid
 AND t.response_payload->>'contract_version'='taskqueue.v2'
 AND (t.response_payload->'messages'='[]'::jsonb
      OR t.response_payload->'messages'='{"_omitted":"content","size":0}'::jsonb)
 AND e.tenant_uuid=t.tenant_uuid AND e.trace_id=t.trace_id
 AND e.record_class IN ('capability_invocation_notification','legacy_unclassified')
 AND e.topic='_topic.integration.gateway.invocation.succeeded'
 AND e.attempts=0 AND e.publish_time IS NULL AND COALESCE(e.delivery_receipt,'')=''
 AND ((e.status='pending' AND e.terminal_at IS NULL)
      OR (e.status='unrouted' AND e.terminal_at<$3))
 AND e.payload->>'tenant_uuid'=t.tenant_uuid AND e.payload->>'trace_id'=t.trace_id
 AND e.payload->>'capability_id'=t.capability_id AND e.payload->>'plugin_id'=t.plugin_id
 AND e.payload->>'status'=t.status
 AND EXISTS(SELECT 1 FROM public.iam_tenant rt WHERE CAST(rt.uuid AS TEXT)=t.tenant_uuid AND rt.deleted_at IS NULL)
 AND (SELECT count(*) FROM ` + traces + ` all_ref WHERE all_ref.event_publication_id=e.uuid)=1 `
