import type { ArchiveJob } from '~/composables/api/services/archiveOpsService';

/** 缺失或未知执行证明不能把历史 running 当作当前活跃。 */
export function archiveExecutionState(job: ArchiveJob): 'active' | 'interrupted' | 'unconfirmed' | 'recorded' {
  if (job.imported || job.recovered) return 'recorded';
  if (job.execution_state === 'active') return 'active';
  if (job.interruption_pending || job.status === 'interrupted') return 'interrupted';
  if (job.status === 'running' || job.status === 'success' && job.remote_state === 'pending') return 'unconfirmed';
  return 'recorded';
}

export function archiveExecutionNeedsReview(job: ArchiveJob): boolean {
  return ['interrupted', 'unconfirmed'].includes(archiveExecutionState(job));
}
