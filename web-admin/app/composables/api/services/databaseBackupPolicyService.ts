import { useApiClient } from "../index";
export interface DatabaseBackupSettings {
  name: string;
  schedule: string;
  retention_count: number;
  retention_days: number;
  retention_mode: "count" | "age_and_count";
  timezone: string;
  drill_enabled: boolean;
  drill_interval_days: number;
  target_ref: string;
}
export interface DatabaseBackupEntry {
  uuid: string;
  revision: number;
  execution_fingerprint: string;
  settings: DatabaseBackupSettings;
  enabled: boolean;
  current: boolean;
}
export interface DatabaseBackupConfiguration {
  authority: "legacy" | "database" | "unavailable";
  state: string;
  ready: boolean;
  desired_revision: number;
  applied_revision: number;
  snapshot_revision: number | null;
  legacy_digest: string;
  policies: DatabaseBackupEntry[];
  execution_ready: Record<string, boolean>;
  problem?: string;
}
export interface DatabaseBackupChange {
  request_id: string;
  expected_revision: number;
  expected_policy_revision: number;
  policy_uuid: string;
  action: "save" | "status" | "current";
  settings?: DatabaseBackupSettings;
  enabled?: boolean;
  confirm: true;
}
export interface DatabaseBackupMutation {
  request_id: string;
  operation_revision: number;
  policy_uuid: string;
  configuration: DatabaseBackupConfiguration;
}
const unwrap = (raw: any) =>
  raw && typeof raw === "object" && "data" in raw ? raw.data : raw;
const uuid = (s: any) =>
  typeof s === "string" &&
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(s);
const count = (n: any) => Number.isSafeInteger(n) && n >= 0;
export function decodeDatabaseBackupConfiguration(
  raw: unknown,
): DatabaseBackupConfiguration {
  const c = unwrap(raw);
  if (
    !c ||
    !["legacy", "database", "unavailable"].includes(c.authority) ||
    ![
      "legacy",
      "pending",
      "applied",
      "failed",
      "unconfirmed",
      "unavailable",
    ].includes(c.state) ||
    typeof c.ready !== "boolean" ||
    ![c.desired_revision, c.applied_revision].every(count) ||
    c.applied_revision > c.desired_revision ||
    !(c.snapshot_revision === null || count(c.snapshot_revision)) ||
    typeof c.legacy_digest !== "string" ||
    !Array.isArray(c.policies) ||
    c.policies.length > 200 ||
    !c.execution_ready ||
    Array.isArray(c.execution_ready) ||
    typeof c.execution_ready !== "object" ||
    !Object.values(c.execution_ready).every((v) => typeof v === "boolean")
  )
    throw new Error("BACKUP_POLICY_RESPONSE_INVALID");
  for (const p of c.policies)
    if (
      !p ||
      !uuid(p.uuid) ||
      !count(p.revision) ||
      typeof p.enabled !== "boolean" ||
      typeof p.current !== "boolean" ||
      !p.settings ||
      typeof p.settings.name !== "string" ||
      typeof p.settings.schedule !== "string" ||
      typeof p.settings.timezone !== "string" ||
      !["count", "age_and_count"].includes(p.settings.retention_mode) ||
      ![
        p.settings.retention_count,
        p.settings.retention_days,
        p.settings.drill_interval_days,
      ].every(count) ||
      typeof p.settings.drill_enabled !== "boolean" ||
      typeof p.settings.target_ref !== "string"
    )
      throw new Error("BACKUP_POLICY_RESPONSE_INVALID");
  return c;
}
export function decodeDatabaseBackupMutation(
  raw: unknown,
): DatabaseBackupMutation {
  const m = unwrap(raw);
  if (
    !m ||
    !uuid(m.request_id) ||
    !count(m.operation_revision) ||
    m.operation_revision < 1 ||
    typeof m.policy_uuid !== "string"
  )
    throw new Error("BACKUP_POLICY_RESPONSE_INVALID");
  decodeDatabaseBackupConfiguration(m.configuration);
  return m;
}
export function useDatabaseBackupPolicyService() {
  const api = useApiClient(),
    base = "/admin/ops/backup/policy-configuration";
  const config = (signal?: AbortSignal) => ({
    signal,
    retry: 0,
    useGlobalLoading: false,
    useGlobalError: false,
    timeout: 15000,
  });
  return {
    async configuration(signal?: AbortSignal) {
      return decodeDatabaseBackupConfiguration(
        await api.get(base, config(signal)),
      );
    },
    async change(input: DatabaseBackupChange, signal?: AbortSignal) {
      return decodeDatabaseBackupMutation(
        await api.post(base + "/changes", input, config(signal)),
      );
    },
    async adopt(
      input: { request_id: string; legacy_digest: string; confirm: true },
      signal?: AbortSignal,
    ) {
      return decodeDatabaseBackupMutation(
        await api.post(base + "/adoptions", input, config(signal)),
      );
    },
    async request(id: string, signal?: AbortSignal) {
      return decodeDatabaseBackupMutation(
        await api.get(
          base + "/requests/" + encodeURIComponent(id),
          config(signal),
        ),
      );
    },
  };
}
