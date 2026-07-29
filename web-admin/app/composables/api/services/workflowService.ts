import { useApiClient } from "../index";
import type { PaginationParams } from "../types/types";

export type WorkflowDefinitionStatus = "draft" | "published" | "archived";
export type WorkflowInstanceState =
  | "queued"
  | "running"
  | "waiting"
  | "succeeded"
  | "failed"
  | "canceled"
  | "compensating";
export type HumanReviewStatus = "pending" | "approved" | "rejected" | "changes_requested" | "canceled";

export interface WorkflowStepDefinition {
  id: string;
  type: "agent" | "system" | "decision" | "parallel" | "human_approval" | "compensation";
  node_kind: string;
  node_ref?: string;
  depends_on?: string[];
  next_step_ids?: string[];
  routes?: Record<string, string[]>;
  input_mapping?: Record<string, any>;
  output_mapping?: Record<string, any>;
  config?: Record<string, any>;
  retry_policy?: Record<string, any>;
  timeout_seconds?: number;
}

export interface WorkflowDefinition {
  uuid: string;
  tenant_uuid: string;
  name: string;
  description?: string;
  version: number;
  status: WorkflowDefinitionStatus;
  step_graph: WorkflowStepDefinition[];
  default_retry_policy?: Record<string, any>;
  compensation_policy?: Record<string, any>;
  sla_policy?: Record<string, any>;
  metadata?: Record<string, any>;
  input_schema?: Record<string, any>;
  workflow_pack_key?: string;
  source_type?: string;
  checksum?: string;
  created_at?: string;
  updated_at?: string;
  published_at?: string | null;
  archived_at?: string | null;
}

export interface CreateWorkflowDefinitionRequest {
  name: string;
  description?: string;
  steps: WorkflowStepDefinition[];
  default_retry_policy?: Record<string, any>;
  compensation_policy?: Record<string, any>;
  sla_policy?: Record<string, any>;
  metadata?: Record<string, any>;
}

export interface ListWorkflowDefinitionParams extends PaginationParams {
  keyword?: string;
  status?: WorkflowDefinitionStatus | WorkflowDefinitionStatus[];
}

export interface NodeSchema {
  type?: string;
  required?: string[];
  properties?: Record<string, any>;
}

export interface WorkflowNodeCatalogItem {
  node_kind: string;
  display_name_i18n_key: string;
  description_i18n_key?: string;
  category: string;
  step_type: string;
  input_schema?: NodeSchema;
  output_schema?: NodeSchema;
  config_schema?: NodeSchema;
  required_permissions?: string[];
  required_capabilities?: string[];
  idempotency_required?: boolean;
  compensation_supported?: boolean;
  source_status: string;
  metadata?: Record<string, any>;
}

export interface WorkflowInstance {
  uuid: string;
  tenant_uuid: string;
  definition_uuid: string;
  definition_version: number;
  state: WorkflowInstanceState;
  trace_id?: string;
  current_step_id?: string;
  last_error?: string;
  started_at?: string | null;
  completed_at?: string | null;
  steps?: WorkflowStepRecord[];
}

export interface WorkflowStepRecord {
  step_id: string;
  type: string;
  node_kind: string;
  node_ref?: string;
  state: string;
  error_code?: string;
  error_message?: string;
  failure_reason?: string;
  payload_in?: Record<string, any> | null;
  payload_out?: Record<string, any> | null;
  scheduled_at?: string | null;
  attempt?: number;
  awaiting_human?: boolean;
  started_at?: string | null;
  completed_at?: string | null;
}

export interface HumanReviewTask {
  review_task_uuid: string;
  tenant_uuid: string;
  workflow_instance_uuid: string;
  step_id: string;
  review_type: string;
  payload?: Record<string, any>;
  approver_policy?: Record<string, any>;
  status: HumanReviewStatus;
  reviewer_user_uuid?: string;
  decision?: string;
  decision_payload?: Record<string, any>;
  comment?: string;
  due_at?: string | null;
  completed_at?: string | null;
}

export interface WorkflowPackSeedRecord {
  workflow_key: string;
  version: number;
  definition_uuid: string;
  definition_version: number;
  checksum: string;
  source: string;
  seeded_at: string;
}

export interface ListResponse<T> {
  items: T[];
  total: number;
}

const toOffset = (params?: PaginationParams) => {
  const pageSize = Number(params?.pageSize || 20);
  const page = Math.max(1, Number(params?.page || 1));
  return {
    page_size: pageSize,
    offset: (page - 1) * pageSize,
  };
};

export const useWorkflowService = () => {
  const apiClient = useApiClient();
  const baseUrl = "/admin/workflows";

  const unwrap = <T>(response: any): T => apiClient.unwrap<T>(response);

  return {
    listDefinitions: async (params?: ListWorkflowDefinitionParams): Promise<ListResponse<WorkflowDefinition>> => {
      const { page_size, offset } = toOffset(params);
      const response = await apiClient.get(`${baseUrl}/definitions`, {
        params: {
          page_size,
          offset,
          keyword: params?.keyword,
          status: params?.status,
        },
      });
      return unwrap<ListResponse<WorkflowDefinition>>(response);
    },

    getDefinition: async (definitionUUID: string): Promise<WorkflowDefinition> => {
      const response = await apiClient.get(`${baseUrl}/definitions/${definitionUUID}`);
      return unwrap<WorkflowDefinition>(response);
    },

    createDefinition: async (data: CreateWorkflowDefinitionRequest): Promise<WorkflowDefinition> => {
      const response = await apiClient.post(`${baseUrl}/definitions`, data);
      return unwrap<WorkflowDefinition>(response);
    },

    validateDefinition: async (definitionUUID: string, steps: WorkflowStepDefinition[]) => {
      const response = await apiClient.post(`${baseUrl}/definitions/${definitionUUID}/validate`, { steps });
      return unwrap<{ valid: boolean; start_step_ids: string[] }>(response);
    },

    listNodeCatalog: async (): Promise<WorkflowNodeCatalogItem[]> => {
      const response = await apiClient.get(`${baseUrl}/node-catalog`);
      return unwrap<{ items: WorkflowNodeCatalogItem[] }>(response).items;
    },

    getNodeCatalogItem: async (nodeKind: string): Promise<WorkflowNodeCatalogItem> => {
      const response = await apiClient.get(`${baseUrl}/node-catalog/${encodeURIComponent(nodeKind)}`);
      return unwrap<WorkflowNodeCatalogItem>(response);
    },

    startInstance: async (definitionUUID: string, input?: Record<string, any>): Promise<WorkflowInstance> => {
      const response = await apiClient.post(`${baseUrl}/instances`, {
        definition_uuid: definitionUUID,
        input: input || {},
      });
      return unwrap<WorkflowInstance>(response);
    },

    getInstance: async (instanceUUID: string, includeSteps = true): Promise<WorkflowInstance> => {
      const response = await apiClient.get(`${baseUrl}/instances/${instanceUUID}`, {
        params: { include_steps: includeSteps ? "true" : "false" },
      });
      return unwrap<WorkflowInstance>(response);
    },

    listInstances: async (params?: PaginationParams & {
      definition_uuid?: string;
      state?: WorkflowInstanceState;
      include_steps?: boolean;
    }): Promise<ListResponse<WorkflowInstance>> => {
      const { page_size, offset } = toOffset(params);
      const page = Math.max(1, Math.floor(offset / page_size) + 1);
      const response = await apiClient.get(`${baseUrl}/instances`, {
        params: {
          page,
          page_size,
          definition_uuid: params?.definition_uuid,
          state: params?.state,
          include_steps: params?.include_steps ? "true" : "false",
        },
      });
      return unwrap<ListResponse<WorkflowInstance>>(response);
    },

    controlInstance: async (
      instanceUUID: string,
      payload: {
        action: "pause" | "resume" | "cancel" | "retry_step" | "trigger_compensation";
        step_id?: string;
        assignment_id?: number;
        reason?: string;
        payload?: Record<string, any>;
      },
    ): Promise<WorkflowInstance> => {
      const response = await apiClient.post(`${baseUrl}/instances/${instanceUUID}/actions`, payload);
      return unwrap<WorkflowInstance>(response);
    },

    listReviewTasks: async (params?: PaginationParams & {
      status?: HumanReviewStatus;
      workflow_instance_uuid?: string;
      review_type?: string;
    }): Promise<ListResponse<HumanReviewTask>> => {
      const response = await apiClient.get(`${baseUrl}/review-tasks`, {
        params: {
          page: params?.page,
          page_size: params?.pageSize,
          status: params?.status,
          workflow_instance_uuid: params?.workflow_instance_uuid,
          review_type: params?.review_type,
        },
      });
      return unwrap<ListResponse<HumanReviewTask>>(response);
    },

    actReviewTask: async (
      reviewTaskUUID: string,
      payload: { action: "approve" | "reject" | "changes_requested"; comment?: string; payload?: Record<string, any> },
    ): Promise<HumanReviewTask> => {
      const response = await apiClient.post(`${baseUrl}/review-tasks/${reviewTaskUUID}/actions`, payload);
      return unwrap<HumanReviewTask>(response);
    },

    listWorkflowPacks: async (params?: PaginationParams & { keyword?: string }): Promise<ListResponse<WorkflowPackSeedRecord>> => {
      const { page_size, offset } = toOffset(params);
      const response = await apiClient.get(`${baseUrl}/packs`, {
        params: {
          page_size,
          offset,
          keyword: params?.keyword,
        },
      });
      return unwrap<ListResponse<WorkflowPackSeedRecord>>(response);
    },

    seedWorkflowPacks: async (keys?: string[]) => {
      const response = await apiClient.post(`${baseUrl}/packs/seed`, { keys: keys || [] });
      return unwrap<{ seeded: WorkflowPackSeedRecord[]; skipped: string[] }>(response);
    },

    getWorkflowPack: async (workflowKey: string): Promise<WorkflowPackSeedRecord> => {
      const response = await apiClient.get(`${baseUrl}/packs/${encodeURIComponent(workflowKey)}`);
      return unwrap<WorkflowPackSeedRecord>(response);
    },

    exportInstances: async (params?: Record<string, any>) => {
      const response = await apiClient.get(`${baseUrl}/instances/export`, { params });
      return unwrap(response);
    },
  };
};
