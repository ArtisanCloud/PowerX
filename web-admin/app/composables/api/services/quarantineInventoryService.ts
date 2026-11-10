import { useApiClient } from "~/composables/api";
import {
  decodeCleanupVerification,
  type BackupCleanupVerification,
} from "./backupCleanupReviewService";
export interface QuarantineInventoryItem {
  request_id: string;
  artifact_id: string;
  job_id: string;
  policy_id: string;
  token: string;
  result: string;
  diagnosis: string;
  reason: string;
  actor_id: string;
  created_at: string;
  completed_at: string | null;
  history_status: string;
  current: string;
  cleanup_state: string;
  size_bytes: number | null;
  can_inspect: boolean;
  snapshot: string;
  valid_backup_proof: false;
}
export interface QuarantineInventoryOptions {
  page: number;
  page_size: number;
  current: string;
  result: string;
  diagnosis: string;
  q: string;
  sort: string;
  direction: string;
}
export interface QuarantineInventoryPage {
  items: QuarantineInventoryItem[];
  total: number;
  page: number;
  page_size: number;
  unattributed_quarantined_artifacts: number;
  observed_at: string;
}
export interface QuarantineInspection {
  request_id: string;
  snapshot: string;
  verification: BackupCleanupVerification;
  currently_quarantined: boolean;
  read_only: true;
  quarantine_unchanged: true;
  valid_backup_proof: false;
}
const invalid = (): never => {
  throw new Error("BACKUP_QUARANTINE_INVENTORY_RESPONSE_INVALID");
};
const uuid = (v: any) =>
  typeof v === "string" &&
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(v) &&
  v !== "00000000-0000-0000-0000-000000000000";
const id = (v: any) => typeof v === "string" && /^[1-9][0-9]{0,19}$/.test(v);
const hash = (v: any) => typeof v === "string" && /^[a-f0-9]{64}$/.test(v);
const date = (v: any) =>
  typeof v === "string" &&
  /^\d{4}-\d{2}-\d{2}T/.test(v) &&
  Number.isFinite(Date.parse(v));
export function decodeQuarantineInventoryItem(
  raw: any,
): QuarantineInventoryItem {
  const v = raw?.data ?? raw;
  if (
    !v ||
    !uuid(v.request_id) ||
    !id(v.artifact_id) ||
    !id(v.job_id) ||
    !id(v.policy_id) ||
    !uuid(v.token) ||
    !id(v.actor_id) ||
    !["checking", "quarantined", "rejected"].includes(v.result) ||
    ![
      "missing",
      "size_changed",
      "checksum_mismatch",
      "sidecars_missing",
      "sidecar_mismatch",
      "metadata_invalid",
    ].includes(v.diagnosis) ||
    typeof v.reason !== "string" ||
    [...v.reason].length > 256 ||
    !date(v.created_at) ||
    (v.result === "checking"
      ? v.completed_at !== null
      : !date(v.completed_at)) ||
    ![
      "covered",
      "sql_replayed",
      "sql_missing",
      "checkpoint_required",
      "independent_missing",
      "conflict",
    ].includes(v.history_status) ||
    ![
      "quarantined",
      "not_quarantined",
      "missing",
      "unavailable",
      "identity_changed",
    ].includes(v.current) ||
    ![
      "",
      "pending",
      "unconfirmed",
      "retained",
      "quarantined",
      "other",
    ].includes(v.cleanup_state) ||
    (v.size_bytes !== null && !Number.isSafeInteger(v.size_bytes)) ||
    typeof v.can_inspect !== "boolean" ||
    typeof v.snapshot !== "string" ||
    v.valid_backup_proof !== false ||
    (v.can_inspect &&
      (!hash(v.snapshot) ||
        !["quarantined", "not_quarantined"].includes(v.current) ||
        v.history_status === "conflict")) ||
    (!v.can_inspect && v.snapshot !== "")
  )
    return invalid();
  return v;
}
export function decodeQuarantineInventory(raw: any): QuarantineInventoryPage {
  const v = raw?.data ?? raw;
  if (
    !v ||
    !Array.isArray(v.items) ||
    !Number.isSafeInteger(v.total) ||
    v.total < 0 ||
    v.total > 4096 ||
    !Number.isSafeInteger(v.page) ||
    v.page < 1 ||
    !Number.isSafeInteger(v.page_size) ||
    v.page_size < 1 ||
    v.page_size > 100 ||
    v.items.length > v.page_size ||
    v.items.length > v.total ||
    !Number.isSafeInteger(v.unattributed_quarantined_artifacts) ||
    v.unattributed_quarantined_artifacts < 0 ||
    !date(v.observed_at)
  )
    return invalid();
  const items = v.items.map(decodeQuarantineInventoryItem);
  if (
    new Set(items.map((r: QuarantineInventoryItem) => r.request_id)).size !==
    items.length
  )
    return invalid();
  return { ...v, items };
}
export async function consumeQuarantineInspection(
  stream: ReadableStream<Uint8Array>,
  row: QuarantineInventoryItem,
  signal: AbortSignal,
  receive: (v: BackupCleanupVerification) => void,
): Promise<QuarantineInspection> {
  const reader = stream.getReader(),
    decoder = new TextDecoder();
  let buffer = "";
  const abort = () => {
    void reader.cancel().catch(() => {});
  };
  signal.addEventListener("abort", abort, { once: true });
  try {
    for (;;) {
      if (signal.aborted)
        throw new Error("BACKUP_QUARANTINE_INSPECTION_INCOMPLETE");
      const { value, done } = await reader.read();
      if (done || signal.aborted)
        throw new Error("BACKUP_QUARANTINE_INSPECTION_INCOMPLETE");
      buffer = (buffer + decoder.decode(value, { stream: true })).replace(
        /\r\n/g,
        "\n",
      );
      if (buffer.length > 1048576) return invalid();
      let end: number;
      while ((end = buffer.indexOf("\n\n")) >= 0) {
        const frame = buffer.slice(0, end);
        buffer = buffer.slice(end + 2);
        const lines = frame.split("\n"),
          event = lines
            .find((l) => l.startsWith("event:"))
            ?.slice(6)
            .trim();
        const data = lines
          .filter((l) => l.startsWith("data:"))
          .map((l) => l.slice(5).trimStart());
        if (!data.length) continue;
        const v = JSON.parse(data.join("\n"));
        if (event === "progress") {
          const p = decodeCleanupVerification(v);
          if (
            p.artifact_id !== row.artifact_id ||
            p.token !== row.token ||
            p.state !== "running"
          )
            return invalid();
          receive(p);
          continue;
        }
        if (event !== "complete")
          throw new Error("BACKUP_QUARANTINE_INSPECTION_INCOMPLETE");
        if (
          v.request_id !== row.request_id ||
          v.snapshot !== row.snapshot ||
          v.read_only !== true ||
          v.quarantine_unchanged !== true ||
          v.valid_backup_proof !== false ||
          v.currently_quarantined !== (row.current === "quarantined")
        )
          return invalid();
        const verification = decodeCleanupVerification(v.verification);
        if (
          verification.artifact_id !== row.artifact_id ||
          verification.token !== row.token ||
          verification.state === "running"
        )
          return invalid();
        return { ...v, verification };
      }
    }
  } finally {
    signal.removeEventListener("abort", abort);
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}
export function useQuarantineInventoryService() {
  const api = useApiClient(),
    config = (signal?: AbortSignal) => ({
      signal,
      retry: 0,
      useGlobalLoading: false,
      useGlobalError: false,
    });
  const base = "/admin/ops/backup/cleanup-quarantine-inventory";
  return {
    async list(o: QuarantineInventoryOptions, signal?: AbortSignal) {
      const q = new URLSearchParams(
        Object.entries(o).map(([k, v]) => [k, String(v)]),
      );
      return decodeQuarantineInventory(
        await api.get(`${base}?${q}`, config(signal)),
      );
    },
    async request(key: string, signal?: AbortSignal) {
      if (!uuid(key)) return invalid();
      const r = decodeQuarantineInventoryItem(
        await api.get(`${base}/${key}`, config(signal)),
      );
      if (r.request_id !== key) return invalid();
      return r;
    },
    async inspect(
      row: QuarantineInventoryItem,
      signal: AbortSignal,
      receive: (v: BackupCleanupVerification) => void,
    ) {
      if (!uuid(row.request_id) || !row.can_inspect || !hash(row.snapshot))
        return invalid();
      const stream = await api.get<ReadableStream<Uint8Array>>(
        `${base}/${row.request_id}/inspection?expected_snapshot=${row.snapshot}`,
        {
          ...config(signal),
          timeout: 0,
          responseType: "stream",
          headers: { Accept: "text/event-stream" },
        },
      );
      return consumeQuarantineInspection(stream, row, signal, receive);
    },
  };
}
