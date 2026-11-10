import type { ArchiveRunOwner } from "./archive-run-request";
export const ARCHIVE_COMMAND_CONFIRMATION_EVENT =
  "powerx:archive-command-confirmation-changed";
export interface ProbeCommandScope {
  target_id: string;
  name: string;
  original_revision: number;
  current_revision: number;
  target_state: "unchanged" | "changed" | "deleted";
  digest: string;
  policies: { id: string; name: string; revision: number; paused: boolean }[];
}
export interface CommandDrainReview {
  subject?: "job" | "probe";
  target_id?: string;
  probe?: ProbeCommandScope;
  record_id: string;
  job_id: string;
  execution_id: string;
  command_id: string;
  kind: string;
  state: string;
  review_version: string;
  eligible: boolean;
  problem?: string;
  boot_matches: boolean;
  leader: string;
  group: string;
  read_only: true;
  observed_at: string;
  identity?: {
    pid: number;
    pgid: number;
    uid: number;
    started: string;
    boot: string;
    namespace: string;
  };
}
export interface CommandConfirmationInput {
  target_id?: string;
  request_id: string;
  expected_version: string;
  execution_id: string;
  command_id: string;
  action: "confirm_local_command_drain";
  reason: string;
  confirm: true;
}
export interface CommandConfirmationReceipt {
  target_id?: string;
  probe?: ProbeCommandScope;
  request_id: string;
  record_id: string;
  job_id: string;
  execution_id: string;
  command_id: string;
  state: "confirmed" | "rejected";
  input_digest: string;
  expected_version: string;
  action: "confirm_local_command_drain";
  reason: string;
  operator: number;
  at: string;
  current_record_matches: boolean;
}
export interface CommandConfirmationIntent {
  target_id?: string;
  version: 1;
  owner: ArchiveRunOwner;
  request_id: string;
  record_id: string;
  job_id: string;
  execution_id: string;
  command_id: string;
  expected_version: string;
  input_digest: string;
  created_at: string;
}
const id = (v: unknown): v is string =>
  typeof v === "string" &&
  /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/.test(v) &&
  v !== "00000000-0000-0000-0000-000000000000";
const hash = (v: unknown): v is string =>
  typeof v === "string" && /^[a-f0-9]{64}$/.test(v);
const date = (v: unknown) =>
  typeof v === "string" && /^\d{4}-/.test(v) && Number.isFinite(Date.parse(v));
const bad = (): never => {
  throw new Error("ARCHIVE_COMMAND_CONFIRMATION_RESPONSE_INVALID");
};
const key = (o: ArchiveRunOwner) =>
  "powerx.archive-command-confirmation.v1:" +
  JSON.stringify([o.sessionId, o.tenantUuid]);
function validSubject(v: any) {
  return v.target_id === undefined || v.target_id === ""
    ? id(v.job_id) && v.job_id === v.record_id
    : v.job_id === "" && id(v.target_id) && id(v.record_id);
}
function validProbe(v: any) {
  return (
    v &&
    id(v.target_id) &&
    typeof v.name === "string" &&
    Number.isSafeInteger(v.original_revision) &&
    v.original_revision >= 0 &&
    Number.isSafeInteger(v.current_revision) &&
    v.current_revision >= 0 &&
    ["unchanged", "changed", "deleted"].includes(v.target_state) &&
    hash(v.digest) &&
    Array.isArray(v.policies) &&
    v.policies.length <= 128 &&
    v.policies.every(
      (p: any) =>
        id(p.id) &&
        typeof p.name === "string" &&
        Number.isSafeInteger(p.revision) &&
        p.revision >= 0 &&
        typeof p.paused === "boolean",
    )
  );
}
export function decodeCommandReview(v: any): CommandDrainReview {
  if (
    !v ||
    !id(v.record_id) ||
    !(
      (v.subject === "probe" &&
        v.job_id === "" &&
        !v.target_id &&
        !v.eligible) ||
      validSubject(v)
    ) ||
    !id(v.execution_id) ||
    !(v.command_id === "" || id(v.command_id)) ||
    !hash(v.review_version) ||
    typeof v.eligible !== "boolean" ||
    typeof v.boot_matches !== "boolean" ||
    typeof v.leader !== "string" ||
    typeof v.group !== "string" ||
    v.read_only !== true ||
    !date(v.observed_at)
  )
    return bad();
  if (v.target_id !== undefined && v.target_id !== "" && !id(v.target_id))
    return bad();
  if (
    (v.subject === "probe" && v.job_id !== "") ||
    (v.target_id &&
      (!validProbe(v.probe) ||
        v.probe.target_id !== v.target_id ||
        v.subject !== "probe")) ||
    (v.eligible &&
      v.subject === "probe" &&
      (!v.target_id || v.probe.policies.some((p: any) => !p.paused)))
  )
    return bad();
  if (
    v.eligible &&
    (!id(v.command_id) ||
      !v.boot_matches ||
      v.leader !== "absent" ||
      v.group !== "absent")
  )
    return bad();
  return v;
}
export function decodeCommandReceipt(v: any): CommandConfirmationReceipt {
  if (
    !v ||
    ![v.request_id, v.record_id, v.execution_id, v.command_id].every(id) ||
    !validSubject(v) ||
    !hash(v.input_digest) ||
    !hash(v.expected_version) ||
    !["confirmed", "rejected"].includes(v.state) ||
    v.action !== "confirm_local_command_drain" ||
    typeof v.reason !== "string" ||
    !Number.isSafeInteger(v.operator) ||
    v.operator < 1 ||
    !date(v.at) ||
    typeof v.current_record_matches !== "boolean"
  )
    return bad();
  if (
    v.state === "confirmed" &&
    v.target_id &&
    (!validProbe(v.probe) ||
      v.probe.target_id !== v.target_id ||
      v.probe.policies.some((p: any) => !p.paused))
  )
    return bad();
  return v;
}
export function readCommandConfirmationIntent(
  storage: Storage,
  owner: ArchiveRunOwner,
): CommandConfirmationIntent | undefined {
  const raw = storage.getItem(key(owner));
  if (raw === null) return;
  if (raw.length > 2300) return bad();
  let v: any;
  try {
    v = JSON.parse(raw);
  } catch {
    return bad();
  }
  if (
    v?.version !== 1 ||
    JSON.stringify(v.owner) !== JSON.stringify(owner) ||
    ![v.request_id, v.record_id, v.execution_id, v.command_id].every(id) ||
    !hash(v.expected_version) ||
    !validSubject(v) ||
    !hash(v.input_digest) ||
    !date(v.created_at)
  )
    return bad();
  return {
    version: 1,
    owner: { ...owner },
    request_id: v.request_id,
    record_id: v.record_id,
    job_id: v.job_id,
    ...(v.target_id ? { target_id: v.target_id } : {}),
    execution_id: v.execution_id,
    command_id: v.command_id,
    expected_version: v.expected_version,
    input_digest: v.input_digest,
    created_at: v.created_at,
  };
}
export function saveCommandConfirmationIntent(
  storage: Storage,
  v: CommandConfirmationIntent,
) {
  if (readCommandConfirmationIntent(storage, v.owner))
    throw new Error("ARCHIVE_COMMAND_CONFIRMATION_PENDING");
  storage.setItem(key(v.owner), JSON.stringify(v));
  const saved = readCommandConfirmationIntent(storage, v.owner);
  if (saved?.request_id !== v.request_id) return bad();
  return saved;
}
export function settleCommandConfirmation(
  storage: Storage,
  i: CommandConfirmationIntent,
  v: CommandConfirmationReceipt,
) {
  if (
    v.request_id !== i.request_id ||
    v.record_id !== i.record_id ||
    v.job_id !== i.job_id ||
    (v.target_id || "") !== (i.target_id || "") ||
    v.execution_id !== i.execution_id ||
    v.command_id !== i.command_id ||
    v.expected_version !== i.expected_version ||
    v.input_digest !== i.input_digest ||
    !(v.state === "rejected" || v.current_record_matches)
  )
    throw new Error("ARCHIVE_COMMAND_CONFIRMATION_UNCONFIRMED");
  const current = readCommandConfirmationIntent(storage, i.owner);
  if (!current || JSON.stringify(current) !== JSON.stringify(i)) return bad();
  storage.removeItem(key(i.owner));
}
export async function commandConfirmationDigest(
  record: string,
  input: CommandConfirmationInput,
) {
  const value = JSON.stringify([
    record,
    {
      request_id: input.request_id,
      expected_version: input.expected_version,
      execution_id: input.execution_id,
      command_id: input.command_id,
      action: input.action,
      reason: input.reason,
      confirm: input.confirm,
      ...(input.target_id ? { target_id: input.target_id } : {}),
    },
  ]).replace(
    /[<>&\u2028\u2029]/g,
    (c) => "\\u" + c.charCodeAt(0).toString(16).padStart(4, "0"),
  );
  return [
    ...new Uint8Array(
      await crypto.subtle.digest("SHA-256", new TextEncoder().encode(value)),
    ),
  ]
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}
