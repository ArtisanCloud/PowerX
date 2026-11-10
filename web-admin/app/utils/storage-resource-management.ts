import type { StoragePolicyOption } from '~/composables/api/services/storageGovernanceService';
import type { StorageRuntime } from '~/composables/api/services/storageRuntimeService';

export type ResourceManagementState = 'automatic' | 'disabled' | 'blocked' | 'observe' | 'module' | 'unknown';
export function resourceManagement(option: StoragePolicyOption, runtime?: StorageRuntime) {
  const kind = option.resource.kind;
  const owner = kind === 'backup_files' ? 'backup' : kind === 'snapshot_files' ? 'snapshot' : kind === 'media_files' ? 'media' : kind === 'archive_objects' ? 'archive' : 'governance';
  const source = runtime?.lifecycle?.sources.find(r => r.id === option.resource.id);
  const policy = runtime?.lifecycle?.policies.find(r => r.source_id === option.resource.id);
  let state: ResourceManagementState;
  if (owner !== 'governance') state = 'module';
  else if (!runtime) state = 'unknown';
  else if (!runtime.configured || !runtime.lifecycle?.configured || !source?.supported) state = 'observe';
  else if (!policy?.enabled || policy.cleanup_mode === 'disabled') state = 'disabled';
  else state = runtime.worker_enabled && runtime.lifecycle?.allow_enforce ? 'automatic' : 'blocked';
  const run = runtime?.lifecycle?.runs.filter(r => r.source_id === option.resource.id).slice().sort((a, b) => Date.parse(b.started_at) - Date.parse(a.started_at))[0];
  const needsReview = !!runtime?.lifecycle?.runs.some(r => r.source_id === option.resource.id && r.state === 'unconfirmed' && !r.acknowledged_at);
  return { owner, state, source, policy, run, needsReview, manual: option.cleanup?.supported === true, canConfigure: owner === 'governance' && runtime?.configured === true && runtime.lifecycle?.configured === true && source?.supported === true };
}
export const resourceDisplayName = (name: string) => name.replace(/（仅观测）$/, '');

export interface RuntimeNavigationRequest { action: 'policy' | 'budget' | 'records'; sourceId?: string }
export interface RuntimeNavigation extends RuntimeNavigationRequest { request: number }
