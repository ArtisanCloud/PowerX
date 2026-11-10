import { useApiClient } from "~/composables/api";
export interface HistoryItem {
  row_id: string;
  request_id: string;
  artifact_id: string;
  actor_id: string;
  state: string;
  coverage: "covered" | "missing" | "conflict" | "invalid";
  problem?: string;
  created_at: string;
}
export interface HistoryPreview {
  snapshot: string;
  source_digest: string;
  observed_at: string;
  total: number;
  foreign: number;
  covered: number;
  missing: number;
  conflicts: number;
  invalid: number;
  unknown: number;
  complete: boolean;
  restore_ready: boolean;
  can_import: boolean;
  pause_ready: boolean;
  problem?: string;
  remaining: number;
  page: number;
  page_size: number;
  filtered: number;
  items: HistoryItem[];
}
export interface HistoryInput {
  request_id: string;
  expected_snapshot: string;
  reason: string;
  confirm: true;
}
export interface HistoryReceipt {
  request_id: string;
  actor_id: string;
  expected_snapshot: string;
  reason: string;
  state: "pending" | "completed" | "rejected";
  problem?: string;
  source_digest?: string;
  total: number;
  covered: number;
  imported: number;
  unknown: number;
  complete: boolean;
  created_at: string;
  completed_at: string | null;
  can_resume: boolean;
}
const hash = (v: any) => typeof v === "string" && /^[a-f0-9]{64}$/.test(v),
  uuid = (v: any) =>
    typeof v === "string" &&
    /^[a-f0-9]{8}-(?:[a-f0-9]{4}-){3}[a-f0-9]{12}$/.test(v) &&
    !/^0{8}-0{4}-0{4}-0{4}-0{12}$/.test(v),
  id = (v: any) => typeof v === "string" && /^[1-9][0-9]{0,19}$/.test(v),
  date = (v: any) => typeof v === "string" && Number.isFinite(Date.parse(v));
const bad = (): never => {
  throw new Error("BACKUP_HISTORY_RESPONSE_INVALID");
};
const data = (v: any) =>
  v && typeof v === "object" && "data" in v ? v.data : v;
const counts = (v: any, keys: string[]) =>
  keys.every((k) => Number.isSafeInteger(v[k]) && v[k] >= 0);
export function decodeHistory(raw: any): HistoryPreview {
  const v = data(raw);
  if (
    !v ||
    !hash(v.snapshot) ||
    !hash(v.source_digest) ||
    !date(v.observed_at) ||
    !counts(v, [
      "total",
      "foreign",
      "covered",
      "missing",
      "conflicts",
      "invalid",
      "unknown",
      "remaining",
      "page",
      "page_size",
      "filtered",
    ]) ||
    v.covered > v.total ||
    v.unknown > v.total ||
    v.filtered > v.total ||
    v.page < 1 ||
    v.page_size < 1 ||
    v.page_size > 50 ||
    !Array.isArray(v.items) ||
    v.items.length > v.page_size ||
    ["complete", "restore_ready", "can_import", "pause_ready"].some(
      (k) => typeof v[k] !== "boolean",
    ) ||
    (v.complete && (v.covered !== v.total || v.conflicts || v.invalid)) ||
    (v.restore_ready && (!v.complete || v.foreign)) ||
    (v.can_import && (!v.pause_ready || v.problem))
  )
    return bad();
  for (const r of v.items)
    if (
      !id(r.row_id) ||
      typeof r.request_id !== "string" ||
      typeof r.artifact_id !== "string" ||
      typeof r.actor_id !== "string" ||
      typeof r.state !== "string" ||
      !["covered", "missing", "conflict", "invalid"].includes(r.coverage) ||
      !date(r.created_at)
    )
      return bad();
  return v;
}
export function decodeHistoryReceipt(raw: any): HistoryReceipt {
  const v = data(raw);
  if (
    !v ||
    !uuid(v.request_id) ||
    !id(v.actor_id) ||
    !hash(v.expected_snapshot) ||
    typeof v.reason !== "string" ||
    [...v.reason].length < 3 ||
    [...v.reason].length > 256 ||
    !["pending", "completed", "rejected"].includes(v.state) ||
    !counts(v, ["total", "covered", "imported", "unknown"]) ||
    v.covered > v.total ||
    v.imported > v.covered ||
    v.unknown > v.covered ||
    typeof v.complete !== "boolean" ||
    typeof v.can_resume !== "boolean" ||
    (v.can_resume && v.state !== "pending") ||
    !date(v.created_at) ||
    (v.state === "pending" && (v.completed_at !== null || v.complete)) ||
    (v.state !== "pending" && !date(v.completed_at)) ||
    v.complete !== (v.state === "completed") ||
    (v.complete && (v.covered !== v.total || !hash(v.source_digest))) ||
    (v.problem !== undefined &&
      (typeof v.problem !== "string" || !/^[A-Z][A-Z0-9_]*$/.test(v.problem)))
  )
    return bad();
  return v;
}
export async function consumeHistoryStream(
  stream: ReadableStream<Uint8Array>,
  input: HistoryInput,
  signal: AbortSignal,
  onReceipt: (r: HistoryReceipt) => void,
) {
  const reader = stream.getReader(),
    decoder = new TextDecoder();
  let buffer = "";
  const abort = () => {
    void reader.cancel().catch(() => {});
  };
  signal.addEventListener("abort", abort, { once: true });
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done || signal.aborted)
        throw new Error("BACKUP_HISTORY_RESULT_UNKNOWN");
      buffer = (buffer + decoder.decode(value, { stream: true })).replace(
        /\r\n/g,
        "\n",
      );
      if (buffer.length > 1048576) return bad();
      let end: number;
      while ((end = buffer.indexOf("\n\n")) >= 0) {
        const frame = buffer.slice(0, end);
        buffer = buffer.slice(end + 2);
        const event = frame.match(/^event:\s*(.*)$/m)?.[1],
          body = frame
            .split("\n")
            .filter((l) => l.startsWith("data:"))
            .map((l) => l.slice(5).trimStart())
            .join("\n");
        if (!body) continue;
        if (event !== "accepted" && event !== "complete")
          throw new Error("BACKUP_HISTORY_RESULT_UNKNOWN");
        const r = decodeHistoryReceipt(JSON.parse(body));
        if (
          r.request_id !== input.request_id ||
          r.expected_snapshot !== input.expected_snapshot ||
          r.reason !== input.reason
        )
          return bad();
        if (event === "complete") {
          if (r.state === "pending") return bad();
          return r;
        }
        if (r.state !== "pending") return bad();
        onReceipt(r);
      }
    }
  } finally {
    signal.removeEventListener("abort", abort);
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}
export function useBackupHistoryService() {
  const api = useApiClient(),
    opts = (signal?: AbortSignal) => ({
      signal,
      retry: 0,
      useGlobalLoading: false,
      useGlobalError: false,
    });
  return {
    async preview(page = 1, coverage = "all", signal?: AbortSignal) {
      return decodeHistory(
        await api.get(
          `/admin/ops/backup/cleanup-retention-history?page=${page}&page_size=20&coverage=${coverage}`,
          opts(signal),
        ),
      );
    },
    async request(key: string, signal?: AbortSignal) {
      if (!uuid(key)) return bad();
      const r = decodeHistoryReceipt(
        await api.get(
          `/admin/ops/backup/cleanup-retention-imports/${key}`,
          opts(signal),
        ),
      );
      if (r.request_id !== key) return bad();
      return r;
    },
    async recent(signal?: AbortSignal) {
      const v: any = data(
        await api.get(
          "/admin/ops/backup/cleanup-retention-imports",
          opts(signal),
        ),
      );
      if (!Array.isArray(v?.items) || v.items.length > 20) return bad();
      return v.items.map(decodeHistoryReceipt) as HistoryReceipt[];
    },
    async import(
      input: HistoryInput,
      signal: AbortSignal,
      accepted: (r: HistoryReceipt) => void,
    ) {
      if (
        !uuid(input.request_id) ||
        !hash(input.expected_snapshot) ||
        input.confirm !== true
      )
        return bad();
      const stream = await api.post<ReadableStream<Uint8Array>>(
        "/admin/ops/backup/cleanup-retention-imports",
        input,
        {
          ...opts(signal),
          timeout: 0,
          responseType: "stream",
          headers: { Accept: "text/event-stream" },
        },
      );
      return consumeHistoryStream(stream, input, signal, accepted);
    },
  };
}
