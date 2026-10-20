import { useApiClient } from "../index";
import { connectBackupEvents } from "~/utils/backup-sse";

export interface RecoveryTarget { id: string; label: string; environment: string; mode: "isolated" | "formal"; allow_overwrite?: boolean; allow_online_data?: boolean }
export interface RecoveryCheck { name: string; passed: boolean; detail: string }
export interface RecoveryJob {
  id: string; kind: "restore" | "initialize" | "capture" | "prepare" | "cutover" | "resume" | "overwrite" | "rollback" | "cleanup"; snapshot_id: string; target_id?: string;
  restore_mode?: "replace_instance" | "new_environment" | "overwrite_current"; validation_url?: string; previous_id?:string; candidate_id?:string;status: string; phase: string; business_verified: boolean; writes_may_have_resumed: boolean;
  rollback_snapshot_id?: string; error?: string; checks: RecoveryCheck[]; created_at: string;
  updated_at?: string;
}
export interface RecoveryOverview { configured: boolean; online: boolean; targets: RecoveryTarget[]; jobs: RecoveryJob[]; progress_tokens?: Record<string, string>; generation: number; maintenance?: boolean; active_environment_id?: string; last_switch_id?: string; recovery_entry?: string; reason?: string }
export interface RecoveryAvailability {
	 reason?: string;
  configured: boolean;
  online: boolean;
  activeCount: number;
  attentionCount: number;
  status: 'loading' | 'unconfigured' | 'offline' | 'unregistered' | 'maintenance' | 'busy' | 'error' | 'ready';
  readySnapshotIDs: string[];
}
export const useSystemRecoveryService = () => {
  const api = useApiClient();
  const base = "/admin/ops/backup/system-snapshots";
  const options = { useGlobalLoading: false, useGlobalError: false };
  return {
    watchProgress(id: string, token: string, change: (job: RecoveryJob) => void, state: (value: string) => void) {
      return connectBackupEvents(async signal => {
        const response = await fetch(`/__powerx_recovery/jobs/${encodeURIComponent(id)}/events`, { signal, headers: { Authorization: `Bearer ${token}`, Accept: "text/event-stream" }, cache: "no-store", credentials: "omit" });
        if (!response.ok || !response.body || !response.headers.get("content-type")?.includes("text/event-stream")) throw new Error(`Progress ${response.status}`);
        return response.body;
      }, event => { if (event.name === "progress" && event.data?.recovery?.id === id) change(event.data.recovery); }, state);
    },
    async overview() { return api.unwrap<RecoveryOverview>(await api.get(`${base}/recovery`, options)); },
    async restore(snapshot: string, generation: number, key: string) {
      return api.unwrap<{ recovery: RecoveryJob; progress_token: string }>(await api.post(`${base}/${encodeURIComponent(snapshot)}/recovery`, { confirmation: snapshot, generation }, { ...options, headers: { "Idempotency-Key": key } }));
    },
    async progress(id: string, token: string): Promise<RecoveryJob> {
      const response = await fetch(`/__powerx_recovery/jobs/${encodeURIComponent(id)}`, { headers: { Authorization: `Bearer ${token}` }, cache: "no-store", credentials: "omit" });
      if (!response.ok) throw new Error(`Progress ${response.status}`);
      return (await response.json()).recovery;
    },
  };
};
