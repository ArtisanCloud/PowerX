import { useApiClient } from "../index";

export interface CapabilityPolicy {
  prefer?: string;
  fallback?: string[];
  rollbackCapability?: string;
}

export interface ProtocolBinding {
  channel: string;
  endpoint?: string;
  schemaRef?: string;
  method?: string;
  rpc?: string;
  toolRef?: string;
  authType?: string;
  healthState?: string;
  lastCheckedAt?: string;
}

export interface WorkflowTemplateRef {
  templateId: string;
  name: string;
  description?: string;
  requiresManualUpgrade: boolean;
  capabilitiesHashSnapshot: string;
  templateHash: string;
  lastSyncedAt?: string;
}

export interface CapabilityRecord {
  capabilityId: string;
  pluginId: string;
  pluginVersion: string;
  title: string;
  description?: string;
  source?: string;
  categories: string[];
  intents: string[];
  toolScope: string[];
  policy?: CapabilityPolicy;
  protocols: ProtocolBinding[];
  workflowTemplates?: WorkflowTemplateRef[];
  compositeGraphs?: unknown;
  annotations?: unknown;
  capabilitiesHash: string;
  protocolHash: string;
  status: string;
  publishedAt?: string;
}

export interface CapabilitySyncJob {
  jobId: string;
  capabilityId?: string;
  pluginId: string;
  pluginVersion: string;
  status: string;
  hashBefore?: string;
  hashAfter?: string;
  errorSummary?: string;
  startedAt?: string;
  finishedAt?: string;
}

export interface PaginationMeta {
  total: number;
  page: number;
  pageSize: number;
  pages?: number;
}

export interface CapabilityListResult {
  items: CapabilityRecord[];
  pagination?: PaginationMeta;
}

export interface CapabilitySyncJobResult {
  items: CapabilitySyncJob[];
  pagination?: PaginationMeta;
}

export interface CapabilityListParams {
  page?: number;
  pageSize?: number;
  pluginId?: string;
  search?: string;
  status?: string[];
  protocol?: string;
  source?: string;
  includeWorkflows?: boolean;
}

export interface CapabilitySyncJobParams {
  page?: number;
  pageSize?: number;
  pluginId?: string;
  capabilityId?: string;
  status?: string[];
}

type CapabilityRecordAPI = {
  capability_id: string;
  plugin_id: string;
  plugin_version: string;
  title: string;
  description?: string;
  source?: string;
  categories?: string[];
  intents?: string[];
  tool_scope?: string[];
  policy?: {
    prefer?: string;
    fallback?: string[];
    rollback_capability_id?: string;
  };
  protocols: Array<{
    channel: string;
    endpoint?: string;
    schema_ref?: string;
    method?: string;
    rpc?: string;
    tool_ref?: string;
    auth_type?: string;
    health_state?: string;
    last_checked_at?: string;
  }>;
  workflow_templates?: Array<{
    template_id: string;
    name: string;
    description?: string;
    requires_manual_upgrade: boolean;
    capabilities_hash_snapshot: string;
    template_hash: string;
    last_synced_at?: string;
  }>;
  composite_graphs?: unknown;
  annotations?: unknown;
  capabilities_hash: string;
  protocol_hash: string;
  status: string;
  published_at?: string;
};

type CapabilitySyncJobAPI = {
  job_id: string;
  capability_id?: string;
  plugin_id: string;
  plugin_version: string;
  status: string;
  hash_before?: string;
  hash_after?: string;
  error_summary?: string;
  started_at?: string;
  finished_at?: string;
};

type ListResponse<T> = {
  items?: T[];
  pagination?: PaginationMeta;
};

const unwrap = <T>(payload: unknown): T => {
  if (payload && typeof payload === "object" && "data" in (payload as any)) {
    return (payload as any).data as T;
  }
  return payload as T;
};

const mapRecord = (record: CapabilityRecordAPI): CapabilityRecord => ({
  capabilityId: record.capability_id,
  pluginId: record.plugin_id,
  pluginVersion: record.plugin_version,
  title: record.title,
  description: record.description,
  source: record.source,
  categories: record.categories ?? [],
  intents: record.intents ?? [],
  toolScope: record.tool_scope ?? [],
  policy: record.policy
    ? {
        prefer: record.policy.prefer,
        fallback: record.policy.fallback ?? [],
        rollbackCapability: record.policy.rollback_capability_id,
      }
    : undefined,
  protocols: (record.protocols || []).map((binding) => ({
    channel: binding.channel,
    endpoint: binding.endpoint,
    schemaRef: binding.schema_ref,
    method: binding.method,
    rpc: binding.rpc,
    toolRef: binding.tool_ref,
    authType: binding.auth_type,
    healthState: binding.health_state,
    lastCheckedAt: binding.last_checked_at,
  })),
  workflowTemplates: record.workflow_templates
    ? record.workflow_templates.map((tpl) => ({
        templateId: tpl.template_id,
        name: tpl.name,
        description: tpl.description,
        requiresManualUpgrade: tpl.requires_manual_upgrade,
        capabilitiesHashSnapshot: tpl.capabilities_hash_snapshot,
        templateHash: tpl.template_hash,
        lastSyncedAt: tpl.last_synced_at,
      }))
    : undefined,
  compositeGraphs: record.composite_graphs,
  annotations: record.annotations,
  capabilitiesHash: record.capabilities_hash,
  protocolHash: record.protocol_hash,
  status: record.status,
  publishedAt: record.published_at,
});

const mapSyncJob = (job: CapabilitySyncJobAPI): CapabilitySyncJob => ({
  jobId: job.job_id,
  capabilityId: job.capability_id,
  pluginId: job.plugin_id,
  pluginVersion: job.plugin_version,
  status: job.status,
  hashBefore: job.hash_before,
  hashAfter: job.hash_after,
  errorSummary: job.error_summary,
  startedAt: job.started_at,
  finishedAt: job.finished_at,
});

const buildQuery = (params: Record<string, any>) => {
  const query: Record<string, any> = {};
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null || value === "") {
      return;
    }
    query[key] = value;
  });
  return query;
};

export class CapabilityRegistryService {
  static async listCapabilities(
    params: CapabilityListParams = {}
  ): Promise<CapabilityListResult> {
    const client = useApiClient();
    const query = buildQuery({
      page: params.page,
      page_size: params.pageSize,
      plugin_id: params.pluginId,
      search: params.search,
      protocol: params.protocol,
      status:
        params.status && params.status.length > 0
          ? params.status.join(",")
          : undefined,
      source: params.source,
      include_workflows: params.includeWorkflows ? "true" : undefined,
    });
    const response = await client.get<ListResponse<CapabilityRecordAPI>>(
      "/admin/capabilities",
      { params: query }
    );
    const data = unwrap<ListResponse<CapabilityRecordAPI>>(response) || {};
    const items = Array.isArray(data.items) ? data.items : [];
    return {
      items: items.map(mapRecord),
      pagination: data.pagination,
    };
  }

  static async listSyncJobs(
    params: CapabilitySyncJobParams = {}
  ): Promise<CapabilitySyncJobResult> {
    const client = useApiClient();
    const query = buildQuery({
      page: params.page,
      page_size: params.pageSize,
      plugin_id: params.pluginId,
      capability_id: params.capabilityId,
      status:
        params.status && params.status.length > 0
          ? params.status.join(",")
          : undefined,
    });
    const response = await client.get<ListResponse<CapabilitySyncJobAPI>>(
      "/admin/capability-sync/jobs",
      { params: query }
    );
    const data = unwrap<ListResponse<CapabilitySyncJobAPI>>(response) || {};
    const items = Array.isArray(data.items) ? data.items : [];
    return {
      items: items.map(mapSyncJob),
      pagination: data.pagination,
    };
  }
}
