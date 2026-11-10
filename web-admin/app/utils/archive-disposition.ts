import type { ArchiveRunOwner } from "./archive-run-request";
export const ARCHIVE_DISPOSITION_EVENT = "powerx:archive-disposition-changed";
export interface DispositionIntent {
  version: 1;
  owner: ArchiveRunOwner;
  request_id: string;
  job_id: string;
  expected_version: string;
  input_digest: string;
  created_at: string;
  closure?: {
    request_id: string;
    expected_version: string;
    input_digest: string;
  };
}
export interface InterruptionReview {
  job_id: string;
  policy_id: string;
  policy_revision: number;
  review_version: string;
  object_count: number;
  bytes: number;
  read_only: true;
  observed_at: string;
}
export interface DispositionInput {
  request_id: string;
  expected_version: string;
  action: "retain_and_close";
  reason: string;
  confirm: true;
}
export interface DispositionReceipt {
  request_id: string;
  job_id: string;
  state: "retained" | "rejected" | "unconfirmed" | "closed_unknown";
  closure?: InterruptionClosure;
  current_protection_matches?: boolean;
  input_digest: string;
  expected_version: string;
  action: "retain_and_close";
  reason: string;
  operator: number;
  created_at: string;
  current_record_matches: boolean;
  result?: {
    request_id: string;
    input_digest: string;
    review_version: string;
    object_count: number;
    bytes: number;
    reason: string;
    operator: number;
    at: string;
  };
  problem?: string;
}
const id = (v: unknown): v is string =>
  typeof v === "string" &&
  /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(v) &&
  v !== "00000000-0000-0000-0000-000000000000";
const hash = (v: unknown): v is string =>
  typeof v === "string" && /^[a-f0-9]{64}$/.test(v);
const date = (v: unknown) =>
  typeof v === "string" && /^\d{4}-/.test(v) && Number.isFinite(Date.parse(v));
const count = (v: unknown) => Number.isSafeInteger(v) && Number(v) >= 0;
const key = (o: ArchiveRunOwner) =>
  "powerx.archive-interruption.v1:" +
  JSON.stringify([o.sessionId, o.tenantUuid]);
const bad = (): never => {
  throw new Error("ARCHIVE_DISPOSITION_RESPONSE_INVALID");
};
export function readDispositionIntent(
  storage: Storage,
  owner: ArchiveRunOwner,
): DispositionIntent | undefined {
  const raw = storage.getItem(key(owner));
  if (raw === null) return;
  if (raw.length > 2048) throw new Error("ARCHIVE_DISPOSITION_CONTEXT_INVALID");
  let v: any;
  try {
    v = JSON.parse(raw);
  } catch {
    return bad();
  }
  if (
    v?.version !== 1 ||
    JSON.stringify(v.owner) !== JSON.stringify(owner) ||
    !id(v.request_id) ||
    !id(v.job_id) ||
    !hash(v.expected_version) ||
    !hash(v.input_digest) ||
    !date(v.created_at)
  )
    return bad();
  if (
    v.closure &&
    (!id(v.closure.request_id) ||
      !hash(v.closure.expected_version) ||
      !hash(v.closure.input_digest))
  )
    return bad();
  return {
    ...(v.closure
      ? {
          closure: {
            request_id: v.closure.request_id,
            expected_version: v.closure.expected_version,
            input_digest: v.closure.input_digest,
          },
        }
      : {}),
    version: 1,
    owner: { ...owner },
    request_id: v.request_id,
    job_id: v.job_id,
    expected_version: v.expected_version,
    input_digest: v.input_digest,
    created_at: v.created_at,
  };
}
export function saveDispositionIntent(storage: Storage, v: DispositionIntent) {
  if (readDispositionIntent(storage, v.owner))
    throw new Error("ARCHIVE_DISPOSITION_PENDING");
  storage.setItem(key(v.owner), JSON.stringify(v));
  const saved = readDispositionIntent(storage, v.owner);
  if (saved?.request_id !== v.request_id)
    throw new Error("ARCHIVE_DISPOSITION_CONTEXT_INVALID");
  return saved;
}
export function settleDisposition(
  storage: Storage,
  intent: DispositionIntent,
  v: DispositionReceipt,
) {
  if (
    v.request_id !== intent.request_id ||
    v.job_id !== intent.job_id ||
    v.input_digest !== intent.input_digest ||
    v.expected_version !== intent.expected_version
  )
    throw new Error("ARCHIVE_DISPOSITION_RECEIPT_MISMATCH");
  if (
    v.state !== "rejected" &&
    !(v.state === "retained" && v.current_record_matches) &&
    !(
      v.state === "closed_unknown" &&
      v.current_protection_matches === true &&
      v.closure &&
      (!intent.closure ||
        (v.closure.request_id === intent.closure.request_id &&
          v.closure.input_digest === intent.closure.input_digest &&
          v.closure.review_version === intent.closure.expected_version))
    )
  )
    throw new Error("ARCHIVE_DISPOSITION_UNCONFIRMED");
  if (
    readDispositionIntent(storage, intent.owner)?.request_id !==
    intent.request_id
  )
    throw new Error("ARCHIVE_DISPOSITION_CONTEXT_INVALID");
  storage.removeItem(key(intent.owner));
}
export async function dispositionDigest(job: string, input: DispositionInput) {
  const data = new TextEncoder().encode(
    JSON.stringify([
      job,
      {
        request_id: input.request_id,
        expected_version: input.expected_version,
        action: input.action,
        reason: input.reason,
        confirm: input.confirm,
      },
    ]).replace(
      /[<>&\u2028\u2029]/g,
      (c) => "\\u" + c.charCodeAt(0).toString(16).padStart(4, "0"),
    ),
  );
  return [...new Uint8Array(await crypto.subtle.digest("SHA-256", data))]
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}
export function decodeInterruptionReview(v: any): InterruptionReview {
  if (
    !v ||
    !id(v.job_id) ||
    !(v.policy_id === "" || id(v.policy_id)) ||
    !count(v.policy_revision) ||
    !hash(v.review_version) ||
    !count(v.object_count) ||
    v.object_count > 10000 ||
    !count(v.bytes) ||
    v.bytes > 64 * 1024 ** 3 ||
    v.read_only !== true ||
    !date(v.observed_at)
  )
    return bad();
  return v;
}
export function decodeDispositionReceipt(v: any): DispositionReceipt {
  if (
    !v ||
    !id(v.request_id) ||
    !id(v.job_id) ||
    !["retained", "rejected", "unconfirmed", "closed_unknown"].includes(
      v.state,
    ) ||
    !hash(v.input_digest) ||
    !hash(v.expected_version) ||
    v.action !== "retain_and_close" ||
    typeof v.reason !== "string" ||
    !count(v.operator) ||
    v.operator < 1 ||
    !date(v.created_at) ||
    typeof v.current_record_matches !== "boolean"
  )
    return bad();
  if (v.state === "closed_unknown") {
    const c = v.closure;
    if (
      !c ||
      !id(c.request_id) ||
      !hash(c.input_digest) ||
      !hash(c.review_version) ||
      !hash(c.registered_epoch) ||
      !hash(c.current_epoch) ||
      !hash(c.job_version) ||
      c.action !== "close_unknown_keep_protection" ||
      typeof c.reason !== "string" ||
      !count(c.operator) ||
      c.operator < 1 ||
      !date(c.at) ||
      typeof v.current_protection_matches !== "boolean"
    )
      return bad();
  }
  if (v.state === "retained") {
    const r = v.result;
    if (
      !r ||
      r.request_id !== v.request_id ||
      r.input_digest !== v.input_digest ||
      r.review_version !== v.expected_version ||
      !count(r.object_count) ||
      r.object_count > 10000 ||
      !count(r.bytes) ||
      !date(r.at) ||
      r.reason !== v.reason ||
      r.operator !== v.operator
    )
      return bad();
  }
  return v;
}
/** 单次只读核验流：EOF／损坏／取消不续读，不生成任何后台请求。 */
export async function consumeInterruptionReview(
  stream: ReadableStream<Uint8Array>,
  job: string,
  signal: AbortSignal,
  progress: (v: { objects: number; bytes: number }) => void,
) {
  const reader = stream.getReader(),
    decoder = new TextDecoder();
  let buffer = "";
  const cancel = () => {
    void reader.cancel().catch(() => {});
  };
  signal.addEventListener("abort", cancel, { once: true });
  try {
    for (;;) {
      if (signal.aborted) throw new Error("ARCHIVE_REVIEW_INCOMPLETE");
      const r = await reader.read();
      if (signal.aborted || r.done)
        throw new Error("ARCHIVE_REVIEW_INCOMPLETE");
      buffer = (buffer + decoder.decode(r.value, { stream: true })).replace(
        /\r\n/g,
        "\n",
      );
      if (buffer.length > 1048576) return bad();
      let end: number;
      while ((end = buffer.indexOf("\n\n")) >= 0) {
        const frame = buffer.slice(0, end);
        buffer = buffer.slice(end + 2);
        const name = frame
          .split("\n")
          .find((l) => l.startsWith("event:"))
          ?.slice(6)
          .trim();
        const data = JSON.parse(
          frame
            .split("\n")
            .filter((l) => l.startsWith("data:"))
            .map((l) => l.slice(5).trimStart())
            .join("\n"),
        );
        if (name === "complete") {
          const v = decodeInterruptionReview(data);
          if (v.job_id !== job) return bad();
          return v;
        }
        if (name === "error")
          throw new Error(
            typeof data.problem === "string"
              ? data.problem
              : "ARCHIVE_REVIEW_INCOMPLETE",
          );
        if (
          name !== "progress" ||
          !count(data.objects) ||
          data.objects > 10000 ||
          !count(data.bytes) ||
          data.bytes > 64 * 1024 ** 3
        )
          return bad();
        progress(data);
      }
    }
  } finally {
    signal.removeEventListener("abort", cancel);
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}

export interface InterruptionRequestReview {
  request_id: string;
  job_id: string;
  input_digest: string;
  review_version: string;
  registered_epoch_matches: boolean;
  interruption_pending: true;
  original_locks_available: true;
  policy_revision: number;
  read_only: true;
  observed_at: string;
}
export interface CloseInterruptionInput {
  request_id: string;
  expected_version: string;
  action: "close_unknown_keep_protection";
  reason: string;
  confirm: true;
}
export interface InterruptionClosure {
  request_id: string;
  input_digest: string;
  review_version: string;
  action: "close_unknown_keep_protection";
  reason: string;
  operator: number;
  at: string;
  registered_epoch: string;
  current_epoch: string;
  job_version: string;
}
export function decodeRequestReview(v: any): InterruptionRequestReview {
  if (
    !v ||
    !id(v.request_id) ||
    !id(v.job_id) ||
    !hash(v.input_digest) ||
    !hash(v.review_version) ||
    typeof v.registered_epoch_matches !== "boolean" ||
    v.interruption_pending !== true ||
    v.original_locks_available !== true ||
    !count(v.policy_revision) ||
    v.read_only !== true ||
    !date(v.observed_at)
  )
    return bad();
  return v;
}
export async function closureDigest(
  original: string,
  inputDigest: string,
  input: CloseInterruptionInput,
) {
  const data = JSON.stringify([
    original,
    inputDigest,
    {
      request_id: input.request_id,
      expected_version: input.expected_version,
      action: input.action,
      reason: input.reason,
      confirm: input.confirm,
    },
  ]).replace(
    /[<>&\u2028\u2029]/g,
    (c) => "\\u" + c.charCodeAt(0).toString(16).padStart(4, "0"),
  );
  return [
    ...new Uint8Array(
      await crypto.subtle.digest("SHA-256", new TextEncoder().encode(data)),
    ),
  ]
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}
export function saveClosureIntent(
  storage: Storage,
  intent: DispositionIntent,
  closure: NonNullable<DispositionIntent["closure"]>,
) {
  const current = readDispositionIntent(storage, intent.owner);
  if (
    !current ||
    current.request_id !== intent.request_id ||
    current.input_digest !== intent.input_digest ||
    current.closure
  )
    throw new Error("ARCHIVE_DISPOSITION_CONTEXT_INVALID");
  storage.setItem(key(intent.owner), JSON.stringify({ ...current, closure }));
  const saved = readDispositionIntent(storage, intent.owner);
  if (saved?.closure?.request_id !== closure.request_id)
    throw new Error("ARCHIVE_DISPOSITION_CONTEXT_INVALID");
  return saved;
}
