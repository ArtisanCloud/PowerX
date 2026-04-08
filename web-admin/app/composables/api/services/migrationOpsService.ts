import { useApiClient } from "../index";

export interface MigrationRunbookRecord {
  id: number | string;
  source_env: string;
  target_env: string;
  status: string;
  db_migration_status: string;
  instance_acceptance_status: string;
  traffic_switch_status: string;
  traffic_rollback_status: string;
  dry_run: boolean;
  summary?: string;
  operator?: string;
  trace_id?: string;
}

export interface TriggerMigrationPayload {
  source_env: string;
  target_env: string;
  dry_run: boolean;
}

export interface MigrationAcceptancePayload {
  db_migration_completed: boolean;
  instance_migration_passed: boolean;
  conclusion?: string;
}

export interface TriggerTrafficSwitchPayload {
  migration_id: string;
  rollback: boolean;
}

const unwrap = <T>(payload: unknown): T => {
  if (payload && typeof payload === "object" && "data" in (payload as any)) {
    return (payload as any).data as T;
  }
  return payload as T;
};

const adminBase = "/admin/migration";

export const useMigrationOpsService = () => {
  const api = useApiClient();

  return {
    async triggerMigration(payload: TriggerMigrationPayload): Promise<MigrationRunbookRecord> {
      const resp = await api.post(`${adminBase}/runbooks/run`, payload);
      const data = unwrap<{ record: MigrationRunbookRecord }>(resp);
      return data.record;
    },

    async getMigration(migrationId: string | number): Promise<MigrationRunbookRecord> {
      const resp = await api.get(`${adminBase}/runbooks/${encodeURIComponent(String(migrationId))}`);
      const data = unwrap<{ record: MigrationRunbookRecord }>(resp);
      return data.record;
    },

    async acceptMigration(migrationId: string | number, payload: MigrationAcceptancePayload): Promise<MigrationRunbookRecord> {
      const resp = await api.post(`${adminBase}/runbooks/${encodeURIComponent(String(migrationId))}/acceptance`, payload);
      const data = unwrap<{ record: MigrationRunbookRecord }>(resp);
      return data.record;
    },

    async triggerTrafficSwitch(payload: TriggerTrafficSwitchPayload): Promise<{ operation_id: string; record: MigrationRunbookRecord }> {
      const resp = await api.post(`${adminBase}/traffic/switch`, payload);
      return unwrap<{ operation_id: string; record: MigrationRunbookRecord }>(resp);
    },
  };
};

