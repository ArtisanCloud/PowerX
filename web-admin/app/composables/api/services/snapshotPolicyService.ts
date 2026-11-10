import { useApiClient } from "../index";
export interface SnapshotPolicySettings {
  name: string;
  interval_minutes: number;
  retention_days: number;
  retention_count: number;
}
export interface SnapshotPolicyEntry {
  id: string;
  revision: number;
  settings: SnapshotPolicySettings;
  enabled: boolean;
  current: boolean;
  anchor: string;
  fingerprint: string;
}
export interface SnapshotPolicyConfiguration {
  authority: "legacy" | "database" | "unavailable";
  state: string;
  ready: boolean;
  desired_revision: number;
  applied_revision: number;
  snapshot_revision: number | null;
  legacy_digest: string;
  policies: SnapshotPolicyEntry[];
  execution_ready: Record<string, boolean>;
  problem?: string;
}
export interface SnapshotPolicyChange {
  request_id: string;
  expected_revision: number;
  expected_policy_revision: number;
  policy_id: string;
  action: "save" | "status" | "current";
  settings?: SnapshotPolicySettings;
  enabled?: boolean;
  confirm: true;
}
export interface SnapshotPolicyMutation {
  request_id: string;
  operation_revision: number;
  policy_id: string;
  configuration: SnapshotPolicyConfiguration;
}
const unwrap = (raw: any) =>
  raw && typeof raw === "object" && "data" in raw ? raw.data : raw;
const count = (n: any) => Number.isSafeInteger(n) && n >= 0;
const uuid = (s: any) =>
  typeof s === "string" &&
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(s);
export function decodeSnapshotPolicyConfiguration(
  raw: unknown,
): SnapshotPolicyConfiguration {
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
    c.policies.length > 100 ||
    !c.execution_ready ||
    Array.isArray(c.execution_ready) ||
    typeof c.execution_ready !== "object" ||
    !Object.values(c.execution_ready).every((v) => typeof v === "boolean")
  )
    throw new Error("SNAPSHOT_POLICY_RESPONSE_INVALID");
  for (const p of c.policies)
    if (
      !p ||
      !uuid(p.id) ||
      !count(p.revision) ||
      typeof p.enabled !== "boolean" ||
      typeof p.current !== "boolean" ||
      !p.settings ||
      typeof p.settings.name !== "string" ||
      ![
        p.settings.interval_minutes,
        p.settings.retention_days,
        p.settings.retention_count,
      ].every((n) => count(n) && n >= 1)
    )
      throw new Error("SNAPSHOT_POLICY_RESPONSE_INVALID");
  return c;
}
export function decodeSnapshotPolicyMutation(
  raw: unknown,
): SnapshotPolicyMutation {
  const m = unwrap(raw);
  if (
    !m ||
    !uuid(m.request_id) ||
    !count(m.operation_revision) ||
    m.operation_revision < 1 ||
    typeof m.policy_id !== "string"
  )
    throw new Error("SNAPSHOT_POLICY_RESPONSE_INVALID");
  decodeSnapshotPolicyConfiguration(m.configuration);
  return m;
}
export function useSnapshotPolicyService() {
  const api = useApiClient(),
    base = "/admin/ops/backup/system-snapshots/policy-configuration";
  const config = (signal?: AbortSignal) => ({
    signal,
    retry: 0,
    useGlobalLoading: false,
    useGlobalError: false,
    timeout: 15000,
  });
  return {
    async configuration(signal?: AbortSignal) {
      return decodeSnapshotPolicyConfiguration(
        await api.get(base, config(signal)),
      );
    },
    async adopt(
      input: { request_id: string; legacy_digest: string; confirm: true },
      signal?: AbortSignal,
    ) {
      return decodeSnapshotPolicyMutation(
        await api.post(base + "/adoptions", input, config(signal)),
      );
    },
    async change(input: SnapshotPolicyChange, signal?: AbortSignal) {
      return decodeSnapshotPolicyMutation(
        await api.post(base + "/changes", input, config(signal)),
      );
    },
    async request(id: string, signal?: AbortSignal) {
      return decodeSnapshotPolicyMutation(
        await api.get(
          base + "/requests/" + encodeURIComponent(id),
          config(signal),
        ),
      );
    },
  };
}
