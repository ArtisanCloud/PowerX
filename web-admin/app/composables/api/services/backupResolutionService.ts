import { useApiClient } from "~/composables/api";
import {
  decodeCleanupVerification,
  type BackupCleanupVerification,
} from "./backupCleanupReviewService";
export interface BackupResolutionPreview {
  item: { artifact_id: string; token: string };
  review_version: string;
  history_digest: string;
  unknown_records: number;
  can_resolve: boolean;
  problem?: string;
  observed_at: string;
}
export interface BackupResolutionInput {
  request_id: string;
  expected_token: string;
  review_version: string;
  history_digest: string;
  reason: string;
  confirm: true;
}
export interface BackupResolutionReceipt {
  request_id: string;
  artifact_id: string;
  token: string;
  review_version: string;
  history_digest: string;
  reason: string;
  state: "checking" | "retained" | "rejected";
  problem?: string;
  actor_id: string;
  created_at: string;
  completed_at: string | null;
  can_resume: boolean;
  independent_complete: boolean;
  sql_committed: boolean;
  original_history_preserved: true;
  recovery_blocked: boolean;
}
const uuid = (v: any) =>
  typeof v === "string" &&
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(v) &&
  !/^0{8}-0{4}-0{4}-0{4}-0{12}$/.test(v);
const id = (v: any) => typeof v === "string" && /^[1-9][0-9]{0,19}$/.test(v);
const date = (v: any) =>
  typeof v === "string" &&
  /^\d{4}-\d{2}-\d{2}T/.test(v) &&
  Number.isFinite(Date.parse(v));
const invalid = (): never => {
  throw new Error("BACKUP_RESOLUTION_RESPONSE_INVALID");
};
export function decodeResolutionReceipt(raw: any): BackupResolutionReceipt {
  const v = raw && typeof raw === "object" && "data" in raw ? raw.data : raw;
  if (
    !v ||
    !uuid(v.request_id) ||
    !id(v.artifact_id) ||
    !uuid(v.token) ||
    typeof v.review_version !== "string" ||
    !/^[a-f0-9]{64}$/.test(v.review_version) ||
    typeof v.reason !== "string" ||
    [...v.reason].length > 256 ||
    !["checking", "retained", "rejected"].includes(v.state) ||
    !id(v.actor_id) ||
    !date(v.created_at) ||
    (v.state === "checking" && v.completed_at !== null) ||
    (v.state !== "checking" && !date(v.completed_at)) ||
    typeof v.can_resume !== "boolean" ||
    (v.can_resume && v.state !== "checking") ||
    v.independent_complete !== (v.state !== "checking") ||
    typeof v.sql_committed !== "boolean" ||
    (v.state === "retained" && !v.sql_committed) ||
    v.original_history_preserved !== true ||
    typeof v.history_digest !== "string" ||
    !/^[a-f0-9]{64}$/.test(v.history_digest) ||
    (v.recovery_blocked !== undefined &&
      typeof v.recovery_blocked !== "boolean") ||
    (v.recovery_blocked === true && v.can_resume) ||
    (v.problem !== undefined &&
      (typeof v.problem !== "string" || !/^[A-Z][A-Z0-9_]*$/.test(v.problem)))
  )
    return invalid();
  return v;
}
export async function consumeResolutionStream(
  stream: ReadableStream<Uint8Array>,
  artifact: string,
  input: BackupResolutionInput,
  signal: AbortSignal,
  receive: (value: {
    receipt?: BackupResolutionReceipt;
    progress?: BackupCleanupVerification;
  }) => void,
): Promise<BackupResolutionReceipt> {
  const reader = stream.getReader(),
    decoder = new TextDecoder();
  let buffer = "";
  const abort = () => {
    void reader.cancel().catch(() => {});
  };
  signal.addEventListener("abort", abort, { once: true });
  try {
    for (;;) {
      if (signal.aborted) throw new Error("BACKUP_RESOLUTION_RESULT_UNKNOWN");
      const { value, done } = await reader.read();
      if (done || signal.aborted)
        throw new Error("BACKUP_RESOLUTION_RESULT_UNKNOWN");
      buffer = (buffer + decoder.decode(value, { stream: true })).replace(
        /\r\n/g,
        "\n",
      );
      if (buffer.length > 1048576) return invalid();
      let end: number;
      while ((end = buffer.indexOf("\n\n")) >= 0) {
        const frame = buffer.slice(0, end);
        buffer = buffer.slice(end + 2);
        let event = "";
        const data: string[] = [];
        for (const line of frame.split("\n")) {
          if (line.startsWith("event:")) event = line.slice(6).trim();
          if (line.startsWith("data:")) data.push(line.slice(5).trimStart());
        }
        if (!data.length) continue;
        const v = JSON.parse(data.join("\n"));
        if (event === "progress") {
          const progress = decodeCleanupVerification(v);
          if (
            progress.artifact_id !== artifact ||
            progress.token !== input.expected_token ||
            progress.state !== "running"
          )
            return invalid();
          receive({ progress });
          continue;
        }
        if (event !== "accepted" && event !== "complete")
          throw new Error("BACKUP_RESOLUTION_RESULT_UNKNOWN");
        const receipt = decodeResolutionReceipt(v);
        if (
          receipt.artifact_id !== artifact ||
          receipt.request_id !== input.request_id ||
          receipt.token !== input.expected_token ||
          receipt.review_version !== input.review_version ||
          receipt.reason !== input.reason ||
          receipt.history_digest !== input.history_digest
        )
          return invalid();
        if (event === "complete") {
          if (receipt.state === "checking" && !receipt.recovery_blocked)
            return invalid();
          return receipt;
        }
        if (receipt.state !== "checking") return invalid();
        receive({ receipt });
      }
    }
  } finally {
    signal.removeEventListener("abort", abort);
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}
export function useBackupResolutionService() {
  const api = useApiClient();
  const config = (signal?: AbortSignal) => ({
    signal,
    retry: 0,
    useGlobalLoading: false,
    useGlobalError: false,
  });
  return {
    async preview(artifact: string, token: string, signal?: AbortSignal) {
      if (!id(artifact) || !uuid(token)) return invalid();
      const raw: any = await api.get(
        `/admin/ops/backup/cleanup-reviews/${artifact}/resolution-preview?expected_token=${token}`,
        config(signal),
      );
      const v = raw?.data ?? raw;
      if (
        !v ||
        v.item?.artifact_id !== artifact ||
        v.item?.token !== token ||
        typeof v.can_resolve !== "boolean" ||
        !Number.isSafeInteger(v.unknown_records) ||
        v.unknown_records < 0 ||
        (v.can_resolve &&
          (!/^[a-f0-9]{64}$/.test(v.review_version) ||
            !/^[a-f0-9]{64}$/.test(v.history_digest))) ||
        !date(v.observed_at)
      )
        return invalid();
      return v as BackupResolutionPreview;
    },
    async list(artifact: string, token: string, signal?: AbortSignal) {
      if (!id(artifact) || !uuid(token)) return invalid();
      const raw: any = await api.get(
        `/admin/ops/backup/cleanup-reviews/${artifact}/resolutions?expected_token=${token}`,
        config(signal),
      );
      const v = raw?.data ?? raw;
      if (!Array.isArray(v?.items) || v.items.length > 20) return invalid();
      const rows = v.items.map(
        decodeResolutionReceipt,
      ) as BackupResolutionReceipt[];
      if (rows.some((r) => r.artifact_id !== artifact || r.token !== token))
        return invalid();
      return rows;
    },
    async request(key: string, signal?: AbortSignal) {
      if (!uuid(key)) return invalid();
      const value = decodeResolutionReceipt(
        await api.get(
          `/admin/ops/backup/cleanup-resolutions/${key}`,
          config(signal),
        ),
      );
      if (value.request_id !== key) return invalid();
      return value;
    },
    async resolve(
      artifact: string,
      input: BackupResolutionInput,
      signal: AbortSignal,
      receive: (value: {
        receipt?: BackupResolutionReceipt;
        progress?: BackupCleanupVerification;
      }) => void,
    ) {
      if (
        !id(artifact) ||
        !uuid(input.request_id) ||
        !uuid(input.expected_token) ||
        !/^[a-f0-9]{64}$/.test(input.review_version) ||
        input.confirm !== true
      )
        return invalid();
      const stream = await api.post<ReadableStream<Uint8Array>>(
        `/admin/ops/backup/cleanup-reviews/${artifact}/resolutions`,
        input,
        {
          ...config(signal),
          timeout: 0,
          responseType: "stream",
          headers: { Accept: "text/event-stream" },
        },
      );
      return consumeResolutionStream(stream, artifact, input, signal, receive);
    },
  };
}
