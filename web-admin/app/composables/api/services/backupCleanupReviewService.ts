import { useApiClient } from "~/composables/api";

export interface BackupCleanupReview {
  review_version?: string; artifact_id: string; job_id: string; policy_id: string;
  state: "pending" | "unconfirmed"; token: string; size_bytes: number;
  updated_at: string; protected: boolean; inspectable: boolean; problem?: string;
}
export interface BackupCleanupReviewPage {
  items: BackupCleanupReview[]; total: number; page: number; page_size: number; observed_at: string;
}
export interface BackupCleanupInspection {
  item: BackupCleanupReview;
  state: "present_unverified" | "missing" | "partial" | "size_changed" | "unsafe" | "unavailable" | "metadata_invalid";
  files: { role: "dump" | "manifest" | "checksum"; state: "present" | "missing" | "unsafe" | "size_changed" | "unavailable"; size_bytes?: number }[];
  observed_at: string; content_verified: false; read_only: true;
}
const id = (v: unknown, zero = false): v is string => typeof v === "string" && (zero && v === "0" || /^[1-9][0-9]{0,19}$/.test(v));
const token = (v: unknown): v is string => typeof v === "string" && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(v) && v !== "00000000-0000-0000-0000-000000000000";
const integer = (v: unknown): v is number => Number.isSafeInteger(v) && (v as number) >= 0;
const date = (v: unknown): v is string => typeof v === "string" && /^\d{4}-\d{2}-\d{2}T/.test(v) && Number.isFinite(Date.parse(v));
const body = (v: any) => v && typeof v === "object" && "data" in v ? v.data : v;
const invalid = (): never => { throw new Error("BACKUP_REVIEW_RESPONSE_INVALID"); };
function item(v: any): BackupCleanupReview {
  if (!v || v.review_version !== undefined && (typeof v.review_version !== "string" || !/^[a-f0-9]{64}$/.test(v.review_version)) || !id(v.artifact_id) || !id(v.job_id, true) || !id(v.policy_id, true) || !["pending", "unconfirmed"].includes(v.state) || typeof v.token !== "string" || v.token.length > 36 || !Number.isSafeInteger(v.size_bytes) || !date(v.updated_at) || typeof v.protected !== "boolean" || typeof v.inspectable !== "boolean" || v.inspectable && !token(v.token) || v.problem !== undefined && (typeof v.problem !== "string" || !/^BACKUP_[A-Z0-9_]+$/.test(v.problem))) return invalid();
  return v as BackupCleanupReview;
}
export function decodeCleanupReviews(raw: unknown): BackupCleanupReviewPage {
  const v = body(raw);
  if (!v || !Array.isArray(v.items) || v.items.length > 100 || !integer(v.total) || !integer(v.page) || v.page < 1 || !integer(v.page_size) || v.page_size < 1 || v.page_size > 100 || !date(v.observed_at)) return invalid();
  return { ...v, items: v.items.map(item) };
}
export function decodeCleanupInspection(raw: unknown): BackupCleanupInspection {
  const v = body(raw);
  if (!v || !["present_unverified", "missing", "partial", "size_changed", "unsafe", "unavailable", "metadata_invalid"].includes(v.state) || !Array.isArray(v.files) || v.files.length > 3 || !date(v.observed_at) || v.content_verified !== false || v.read_only !== true) return invalid();
  const roles = new Set<string>();
  for (const f of v.files) {
    if (!f || !["dump", "manifest", "checksum"].includes(f.role) || roles.has(f.role) || !["present", "missing", "unsafe", "size_changed", "unavailable"].includes(f.state) || f.size_bytes !== undefined && !integer(f.size_bytes)) return invalid();
    roles.add(f.role);
  }
  return { ...v, item: item(v.item) };
}
export function useBackupCleanupReviewService() {
  const api = useApiClient();
  const config = (signal?: AbortSignal) => ({ signal, retry: 0, useGlobalLoading: false, useGlobalError: false });
  return {
    async list(page = 1, signal?: AbortSignal) { return decodeCleanupReviews(await api.get(`/admin/ops/backup/cleanup-reviews?page=${page}&page_size=20`, config(signal))); },
    async verify(artifact: string, expected: string, signal: AbortSignal, progress: (v: BackupCleanupVerification) => void) {
      if (!id(artifact) || !token(expected) || signal.aborted) throw new Error('BACKUP_REVIEW_INTENT_INVALID');
      const stream = await api.get<ReadableStream<Uint8Array>>(`/admin/ops/backup/cleanup-reviews/${artifact}/verification?expected_token=${expected}`, { ...config(signal), timeout: 0, responseType: 'stream', headers: { Accept: 'text/event-stream' } });
      return consumeCleanupVerification(stream, artifact, expected, signal, progress);
    },
    async inspect(artifact: string, expected: string, signal?: AbortSignal) {
      if (!id(artifact) || !token(expected)) throw new Error("BACKUP_REVIEW_INTENT_INVALID");
      return decodeCleanupInspection(await api.get(`/admin/ops/backup/cleanup-reviews/${artifact}?expected_token=${expected}`, config(signal)));
    },
  };
}

export interface BackupCleanupVerification {
  artifact_id: string; token: string;
  state: 'running' | 'verified' | 'checksum_mismatch' | 'sidecar_mismatch' | 'sidecars_missing' | 'missing' | 'unsafe' | 'metadata_invalid' | 'size_changed' | 'changed' | 'unavailable' | 'interrupted';
  bytes_read: number; total_bytes: number; dump_matched: boolean; content_verified: boolean;
  read_only: true; cleanup_unchanged: true; observed_at: string;
}
export function decodeCleanupVerification(v: any): BackupCleanupVerification {
  if (!v || !id(v.artifact_id) || !token(v.token) || !['running','verified','checksum_mismatch','sidecar_mismatch','sidecars_missing','missing','unsafe','metadata_invalid','size_changed','changed','unavailable','interrupted'].includes(v.state) || !integer(v.bytes_read) || !integer(v.total_bytes) || v.bytes_read > v.total_bytes || typeof v.dump_matched !== 'boolean' || typeof v.content_verified !== 'boolean' || v.read_only !== true || v.cleanup_unchanged !== true || !date(v.observed_at) || v.content_verified !== (v.state === 'verified') || v.state === 'verified' && (!v.dump_matched || v.bytes_read !== v.total_bytes || v.total_bytes === 0)) return invalid();
  return v;
}

/** One explicitly requested stream. EOF, errors or cancellation never reconnect or retry the read. */
export async function consumeCleanupVerification(stream: ReadableStream<Uint8Array>, artifact: string, expected: string, signal: AbortSignal, progress: (v: BackupCleanupVerification) => void): Promise<BackupCleanupVerification> {
  const reader = stream.getReader(), decoder = new TextDecoder(); let buffer = '';
  const cancel = () => { void reader.cancel().catch(() => {}); };
  signal.addEventListener('abort', cancel, { once: true });
  try {
    for (;;) {
      if (signal.aborted) throw new Error('BACKUP_VERIFICATION_INCOMPLETE');
      const { value, done } = await reader.read();
      if (signal.aborted || done) throw new Error('BACKUP_VERIFICATION_INCOMPLETE');
      buffer = (buffer + decoder.decode(value, { stream: true })).replace(/\r\n/g, '\n');
      if (buffer.length > 1048576) return invalid();
      let end: number;
      while ((end = buffer.indexOf('\n\n')) >= 0) {
        const frame = buffer.slice(0, end); buffer = buffer.slice(end + 2); let name = ''; const data: string[] = [];
        for (const line of frame.split('\n')) { if (line.startsWith('event:')) name = line.slice(6).trim(); if (line.startsWith('data:')) data.push(line.slice(5).trimStart()); }
        if (!data.length) continue;
        if (!['progress','complete'].includes(name)) throw new Error('BACKUP_VERIFICATION_INCOMPLETE');
        const v = decodeCleanupVerification(JSON.parse(data.join('\n')));
        if (v.artifact_id !== artifact || v.token !== expected || signal.aborted) return invalid();
        if (name === 'complete') { if (v.state === 'running') return invalid(); return v; }
        if (v.state !== 'running') return invalid();
        progress(v);
      }
    }
  } finally { signal.removeEventListener('abort', cancel); await reader.cancel().catch(() => {}); reader.releaseLock(); }
}
