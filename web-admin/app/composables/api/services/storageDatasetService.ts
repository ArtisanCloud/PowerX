import { useApiClient } from '../index';

export interface StorageDataset {
  dataset_uuid: string;
  dataset_key: string;
  name: string;
  owner_kind: 'system' | 'plugin';
  plugin_id?: string;
  source_state: 'ready' | 'unavailable' | 'needs_revalidation';
  acceptance_revision?: number;
  observation: { total_bytes: number | null };
  available_actions: ('observe' | 'preview' | 'delete' | 'archive' | 'open_existing_policy')[];
  retention_bounds?: { minimum_days: number; suggested_days: number; maximum_days: number };
}

export interface DatasetPreviewInput {
  request_uuid: string;
  tenant_uuid: string;
  expected_acceptance_revision: number;
  retention_days: number;
}

export interface DatasetPreview {
  request_uuid: string;
  dataset_uuid: string;
  state: 'queued' | 'running' | 'completed' | 'failed' | 'invalidated' | 'expired';
  cutoff_at: string;
  result?: {
    coverage: 'complete' | 'bounded';
    examined_count: number;
    candidate_count: number;
    protected_by_reason: { reason: string; count: number }[];
  };
}

export type DatasetAction = 'delete' | 'archive';

export interface DatasetExecutionPolicy {
  revision: number;
  acceptance_revision: number;
  state: 'enabled' | 'paused' | 'draining';
  enabled: boolean;
  action?: DatasetAction;
  retention_days?: number;
  archive_target_uuid?: string;
  active_request_uuid?: string;
}

export interface DatasetRunInput {
  request_uuid: string;
  tenant_uuid: string;
  preview_request_uuid: string;
  expected_policy_revision: number;
}

export interface DatasetRun {
  request_uuid: string;
  state: 'queued' | 'authorized' | 'needs_reconciliation' | 'committed' | 'aborted' | 'cancelled';
  affected_count?: number;
  archive_available?: boolean;
}

type PolicyChange = { expected_revision: number; enabled: boolean; acceptance_revision?: number; action?: DatasetAction; retention_days?: number; archive_target_uuid?: string };

interface DatasetPage { items: StorageDataset[]; next_cursor: string | null; page_physical_bytes: number | null }
const endpoint = '/admin/ops/storage/datasets';
const datasetPath = (id: string) => `${endpoint}/${encodeURIComponent(id)}`;
const opts = (signal?: AbortSignal) => ({ signal, retry: 0, useGlobalLoading: false });

function data<T>(raw: T | { data: T }): T {
  return raw && typeof raw === 'object' && 'data' in raw ? raw.data as T : raw as T;
}

export function useStorageDatasetService() {
  const api = useApiClient();
  const policyPath = (id: string, tenant: string) => `${datasetPath(id)}/policies/${encodeURIComponent(tenant)}`;
  const runPath = (id: string, request: string) => `${datasetPath(id)}/runs/${encodeURIComponent(request)}`;
  return {
    async list(owner: string, cursor: string, signal: AbortSignal): Promise<DatasetPage> {
      const params = new URLSearchParams({ limit: '25' });
      if (owner) params.set('owner_kind', owner);
      if (cursor) params.set('cursor', cursor);
      return data(await api.get<DatasetPage>(`${endpoint}?${params}`, opts(signal)));
    },
    async createPreview(id: string, input: DatasetPreviewInput, signal: AbortSignal): Promise<DatasetPreview> {
      return data(await api.post<DatasetPreview>(`${datasetPath(id)}/previews`, input, opts(signal)));
    },
    async preview(id: string, request: string, signal: AbortSignal): Promise<DatasetPreview> {
      return data(await api.get<DatasetPreview>(`${datasetPath(id)}/previews/${encodeURIComponent(request)}`, opts(signal)));
    },
    async policy(id: string, tenant: string, signal?: AbortSignal) {
      return data(await api.get<DatasetExecutionPolicy>(policyPath(id, tenant), opts(signal)));
    },
    async changePolicy(id: string, tenant: string, change: PolicyChange, signal?: AbortSignal) {
      return data(await api.put<DatasetExecutionPolicy>(policyPath(id, tenant), change, opts(signal)));
    },
    async startRun(id: string, input: DatasetRunInput, signal?: AbortSignal) {
      return data(await api.post<DatasetRun>(`${datasetPath(id)}/runs`, input, opts(signal)));
    },
    async run(id: string, request: string, signal?: AbortSignal) {
      return data(await api.get<DatasetRun>(runPath(id, request), opts(signal)));
    },
    async cancelRun(id: string, request: string, signal?: AbortSignal) {
      return data(await api.post<DatasetRun>(`${runPath(id, request)}/cancellations`, {}, opts(signal)));
    },
    async download(id: string, request: string, signal?: AbortSignal) {
      return api.get<Blob>(`${runPath(id, request)}/archive`, { ...opts(signal), responseType: 'blob' });
    },
  };
}
