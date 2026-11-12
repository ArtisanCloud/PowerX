export interface PluginInstallTask {
  task_id: string; request_id: string; plugin_id: string; version: string;
  installed_revision?: string; source_kind: 'local' | 'url'; revision: number;
  status: 'awaiting_upload' | 'queued' | 'running' | 'succeeded' | 'failed' | 'needs_attention' | 'cancelled';
  phase: string; error_code?: string; failure_details?: Record<string, any>;
  created_at: string; updated_at: string; completed_at?: string;
}
export const installTaskTerminal = (task: PluginInstallTask) => ['succeeded', 'failed', 'needs_attention', 'cancelled'].includes(task.status);
export function acceptInstallTask(previous: PluginInstallTask, next: PluginInstallTask): boolean {
  return !!next && next.task_id === previous.task_id && Number.isSafeInteger(next.revision) && next.revision >= previous.revision &&
    (!installTaskTerminal(previous) || (next.status === previous.status && next.revision === previous.revision));
}
