// 仅保留当前登录会话的归档请求身份，不保存令牌、凭据或恢复票据。
export const ARCHIVE_RUN_REQUEST_EVENT = 'powerx:archive-run-request-changed';
export interface ArchiveRunOwner { sessionId: string; tenantUuid: string | null }
export interface ArchiveRunIntent {
  version: 1;
  owner: ArchiveRunOwner;
  request_id: string;
  policy_id: string;
  policy_revision: number;
  source_id: string;
  created_at: string;
}
export interface ArchiveRunReceipt {
  request_id: string; policy_id: string; job_id: string; policy_revision: number;
  source_id: string; status: string; phase: string; remote_state: string;
  created_at: string; updated_at: string;
}
const uuid = (v: unknown): v is string => typeof v === 'string' && /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(v) && v !== '00000000-0000-0000-0000-000000000000';
const text = (v: unknown, max = 128): v is string => typeof v === 'string' && v.length > 0 && v.length <= max;
const revision = (v: unknown) => Number.isSafeInteger(v) && Number(v) > 0;
const date = (v: unknown) => typeof v === 'string' && v.length <= 40 && Number.isFinite(Date.parse(v));
export const archiveRunOwnerKey = (owner: ArchiveRunOwner) => JSON.stringify([owner.sessionId, owner.tenantUuid]);
const key = (owner: ArchiveRunOwner) => 'powerx.archive-run-intent.v1:' + archiveRunOwnerKey(owner);
const validOwner = (owner: any): owner is ArchiveRunOwner => owner && text(owner.sessionId, 256) && (owner.tenantUuid === null || text(owner.tenantUuid, 256));

export function decodeArchiveRunReceipt(value: any): ArchiveRunReceipt {
  if (!value || !uuid(value.request_id) || !uuid(value.policy_id) || !uuid(value.job_id) || !revision(value.policy_revision) || !text(value.source_id) || !text(value.status) || !text(value.phase) || !text(value.remote_state) || !date(value.created_at) || !date(value.updated_at)) throw new Error('ARCHIVE_RUN_RECEIPT_INVALID');
  return { request_id: value.request_id, policy_id: value.policy_id, job_id: value.job_id, policy_revision: value.policy_revision, source_id: value.source_id, status: value.status, phase: value.phase, remote_state: value.remote_state, created_at: value.created_at, updated_at: value.updated_at };
}

export function readArchiveRunIntent(storage: Storage, owner: ArchiveRunOwner): ArchiveRunIntent | undefined {
  if (!validOwner(owner)) throw new Error('ARCHIVE_RUN_SESSION_REQUIRED');
  const raw = storage.getItem(key(owner));
  if (raw === null) return;
  if (raw.length > 2048) throw new Error('ARCHIVE_RUN_CONTEXT_UNREADABLE');
  let p: any;
  try { p = JSON.parse(raw); } catch { throw new Error('ARCHIVE_RUN_CONTEXT_UNREADABLE'); }
  if (!p || p.version !== 1 || !validOwner(p.owner) || archiveRunOwnerKey(p.owner) !== archiveRunOwnerKey(owner) || !uuid(p.request_id) || !uuid(p.policy_id) || !revision(p.policy_revision) || !text(p.source_id) || !date(p.created_at)) throw new Error('ARCHIVE_RUN_CONTEXT_UNREADABLE');
  // 只复制固定字段，浏览器缓存中的额外字段不能成为 API 输入。
  return { version: 1, owner: { ...owner }, request_id: p.request_id, policy_id: p.policy_id, policy_revision: p.policy_revision, source_id: p.source_id, created_at: p.created_at };
}

export function beginArchiveRunIntent(storage: Storage, intent: ArchiveRunIntent): ArchiveRunIntent {
  if (readArchiveRunIntent(storage, intent.owner)) throw new Error('ARCHIVE_RUN_REQUEST_PENDING');
  // 在写入前用相同解码规则验证；写入失败时调用方禁止 POST。
  const raw = JSON.stringify(intent);
  const candidate = readArchiveRunIntent({ getItem: () => raw } as Storage, intent.owner)!;
  storage.setItem(key(intent.owner), JSON.stringify(candidate));
  const saved = readArchiveRunIntent(storage, intent.owner);
  if (!saved || saved.request_id !== candidate.request_id) throw new Error('ARCHIVE_RUN_CONTEXT_UNREADABLE');
  return saved;
}

export function settleArchiveRunIntent(storage: Storage, intent: ArchiveRunIntent, value: unknown): ArchiveRunReceipt {
  const receipt = decodeArchiveRunReceipt(value);
  if (receipt.request_id !== intent.request_id || receipt.policy_id !== intent.policy_id || receipt.policy_revision !== intent.policy_revision || receipt.source_id !== intent.source_id) throw new Error('ARCHIVE_RUN_RECEIPT_MISMATCH');
  const saved = readArchiveRunIntent(storage, intent.owner);
  if (!saved || saved.request_id !== intent.request_id || saved.policy_id !== intent.policy_id || saved.policy_revision !== intent.policy_revision || saved.source_id !== intent.source_id) throw new Error('ARCHIVE_RUN_CONTEXT_CHANGED');
  storage.removeItem(key(intent.owner));
  if (storage.getItem(key(intent.owner)) !== null) throw new Error('ARCHIVE_RUN_CONTEXT_UNREADABLE');
  return receipt;
}
