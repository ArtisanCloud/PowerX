# Scheduler delivery through TaskQueue v2

Plugin jobs can explicitly request durable delivery with this scheduler payload:

```json
{
  "business_action": "ecommerce-order-timeout-close",
  "task_dispatch": {
    "contract": "scheduler.taskqueue.v1",
    "subscriber_id": "runtime.scheduler.jobs"
  }
}
```

The scheduler persists a complete trigger snapshot in the same transaction as its run. The snapshot's tenant, plugin owner and job identity come from the persisted job. Caller-supplied delivery payloads and unknown dispatch options are rejected. Job edits do not rewrite an issued run.

Core resolves the exact enabled `auth.credentials` record for that tenant and plugin. Its live client namespace must match the Framework STS consumer namespace. The credential must declare `com.corex.runtime.taskqueue.service_consume`. Consumer requests still pass the published typed service capability and live grant checks; this feature adds no Admin proxy or alternate authentication path.

A run uses its own UUID for TaskQueue idempotency. Publication happens after the trigger transaction commits and under the same job publication fence used by deletion. A failed enqueue remains `pending_dispatch` and retries with bounded backoff; five failed attempts become `failed`. Paused or deleted jobs stop pending delivery. `queued` means that TaskQueue accepted delivery, and does not mean that the business handler completed. Actual execution and fenced receipt outcomes remain in TaskQueue history.

The plugin uses Framework's blocking dequeue, renews active leases, and settles the current receipt after its handler completes. It validates the trigger tenant, owner, topic, job name and business action. It must not add a second local scheduler in Host mode.

Jobs without `task_dispatch` retain their existing event publication behavior. Ecommerce currently opts its order timeout and payment reconciliation workers into this contract. Other workers and plugins require their own explicit adoption and acceptance.

## Schema and validation

Run fields: `dispatch_payload_json`, `dispatch_attempts`, `next_dispatch_at`; index: `idx_scheduler_run_dispatch`. Model migration remains registered in `pkg/corex/db/database/migration.go`. For development or test databases, the narrow command is:

```sh
cd backend
go run ./cmd/database migrate -config etc/config.yaml -scope scheduler-task-dispatch
```

The narrow migration only adds these fields and index to an existing scheduler run table. It does not replay historical runs, backfill delivery snapshots, change business records, or seed permissions. Installed environments use the formal full upgrade plan.

Targeted tests cover immutable retry snapshots, separate identities for recurring runs, deletion fencing, paused jobs, bounded failure, exact tenant and STS namespace binding, missing/disabled grants, and a repeatable schema migration that preserves legacy records in SQLite and an isolated PostgreSQL schema.
