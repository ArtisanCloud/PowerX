import { useApiClient } from "../index";
import type { ApiResponse } from "../types/types";

export interface EventFabricTopic {
  id: string;
  uuid?: string;
  scope_type?: "system" | "tenant" | "plugin" | "third_party";
  scope_id?: string;
  tenant_uuid?: string;
  tenant_key?: string;
  full_topic: string;
  namespace: string;
  name: string;
  lifecycle: string;
  payload_format: string;
  max_retry: number;
  ack_timeout_sec: number;
  versioning_mode: string;
  created_at?: string;
  updated_at?: string;
  deprecated_at?: string;
}

export interface EventFabricGroupedCount {
  topic_uuid: string;
  full_topic?: string;
  by_status: Record<string, number>;
  total: number;
}

export interface EventFabricReplayTask {
  id: string;
  topic_uuid: string;
  full_topic?: string;
  trace_id?: string;
  status: string;
  shadow: boolean;
  requested_by?: string;
  submitted_at: string;
  completed_at?: string;
  failure_reason?: string;
  result_count: number;
}

export interface EventFabricOverview {
  now: string;
  tenant_uuid: string;
  filters: {
    namespace?: string;
    name?: string;
    subscriber_id?: string;
    replay_task_max?: number;
  };
  topics: EventFabricTopic[];
  stats: {
    dlq: {
      by_topic: EventFabricGroupedCount[];
      total: number;
    };
    delivery_attempts: {
      subscriber_id: string;
      by_topic: EventFabricGroupedCount[];
      total: number;
    };
    replay_tasks: {
      recent: EventFabricReplayTask[];
    };
  };
}

export interface EventFabricDlqMessage {
  id: string;
  tenant_uuid: string;
  topic: string;
  event_id: string;
  retry_count: number;
  last_error?: string;
  failed_at: string;
}

export interface EventFabricDlqListResult {
  items: EventFabricDlqMessage[];
  pagination: {
    total: number;
    page: number;
    page_size: number;
  };
}

export interface EventFabricReplayTaskResponse {
  id: string;
  tenant_uuid: string;
  topic: string;
  trace_id: string;
  status: string;
  shadow: boolean;
  requested_by: string;
  submitted_at: string;
  completed_at?: string;
  failure_reason?: string;
  result_count: number;
}

export interface EventFabricCronJob {
  id: string;
  name: string;
  description: string;
  status: string;
  kind: string;
  interval_sec?: number;
  batch_size?: number;
  subscriber_id?: string;
  tenant_key?: string;
  next_run_at?: string;
  supports_pause: boolean;
  supports_run_now: boolean;
}

export interface EventFabricCronJobListResult {
  items: EventFabricCronJob[];
  now: string;
}

export interface EventFabricTaskQueueSubscriberStats {
  subscriber_id: string;
  tenant_key: string;
  pending: number;
  deferred: number;
  processing: number;
  inflight: number;
  total_tasks?: number;
}

export interface EventFabricTaskQueueStats {
  pending: number;
  deferred: number;
  processing: number;
  inflight: number;
  by_subscriber: EventFabricTaskQueueSubscriberStats[];
}

export interface EventFabricTaskQueueStatsResult {
  now: string;
  tenant_uuid: string;
  subscriber_id: string;
  task_queue: EventFabricTaskQueueStats;
}

export interface EventFabricTaskQueueMessage {
  id: string;
  topic: string;
  trace_id?: string;
  attempt: number;
  visible_at?: string;
  metadata?: Record<string, string>;
}

export interface EventFabricTaskQueueMessagesResult {
  now: string;
  tenant_uuid: string;
  tenant_key: string;
  subscriber_id: string;
  limit: number;
  messages: {
    pending: EventFabricTaskQueueMessage[];
    deferred: EventFabricTaskQueueMessage[];
    processing: EventFabricTaskQueueMessage[];
    inflight: EventFabricTaskQueueMessage[];
  };
  history: Array<{
    task_id: string;
    tenant_key: string;
    subscriber_id: string;
    topic: string;
    kind: string;
    status: string;
    trace_id?: string;
    attempt: number;
    source: string;
    submitted_at?: string;
    completed_at?: string;
    last_seen_at?: string;
  }>;
}

export interface EventFabricPipelineDebugPayload {
  title?: string;
  content?: string;
  type?: string;
  category?: string;
  isImportant?: boolean;
  metadata?: Record<string, any>;
  topic?: string;
  subscriber_id?: string;
  tenant_key?: string;
}

export interface EventFabricPipelineDebugResult {
  task_id: string;
  subscriber_id: string;
  topic: string;
  tenant_key?: string;
  payload: Record<string, any>;
}

export interface EventFabricRetrySeedResult {
  event_id: string;
  delivery_id: string;
  topic: string;
  subscriber_id: string;
  tenant_key: string;
  retry_after_seconds: number;
  retry_at: string;
  remaining_attempts: number;
  max_attempts: number;
  trace_id?: string;
}

export interface EventFabricRetryTaskStatus {
  delivery_id: string;
  event_id: string;
  topic: string;
  subscriber_id: string;
  tenant_key: string;
  status: string;
  last_error_code?: string;
  nack_reason?: string;
  scheduled_at?: string;
  last_attempt_at?: string;
  acked_at?: string;
}



export interface EventFabricCreateTopicPayload {
  namespace: string;
  name: string;
  payload_format?: string;
  max_retry?: number;
  ack_timeout_sec?: number;
  versioning_mode?: string;
  retention_policy?: string;
  metadata?: Record<string, any>;
  created_by?: string;
}

export interface EventFabricUpdateLifecyclePayload {
  target_state: "active" | "deprecated" | "retired";
  change_reason?: string;
}

export interface EventFabricAclBinding {
	id: string;
	tenant_uuid: string;
	tenant_key: string;
	topic_uuid: string;
	principal_type: string;
	principal_id: string;
	action: string;
	expires_at?: string;
	granted_by?: string;
	justification?: string;
	audit_ref?: string;
}

export interface EventFabricAclTopicMatrixResult {
	topic: {
		topic_uuid: string;
		tenant_key: string;
		full_topic: string;
		namespace: string;
		name: string;
	};
	actions: string[];
	principals: Array<{
		principal_id: string;
		actions: Record<string, boolean>;
	}>;
}

export interface EventFabricAclPrincipalMatrixResult {
	principal_id: string;
	topics: Array<{
		topic_uuid: string;
		topic: string;
		actions: string[];
	}>;
}

export const useEventFabricService = () => {
  const apiClient = useApiClient();
  // Event Fabric 监管属于 Admin 能力（Root 可见），后端路由在 /api/v1/admin/event-fabric/*
  const baseUrl = "/admin/event-fabric";

  return {
    getOverview: (params?: {
      namespace?: string;
      name?: string;
      subscriber_id?: string;
      limit?: number;
    }) => {
      return apiClient.get<ApiResponse<EventFabricOverview>>(
        `${baseUrl}/overview`,
        { params }
      );
    },

    listTopics: (params?: {
      namespace?: string;
      lifecycle?: string;
      tenant_uuid?: string;
      page?: number;
      page_size?: number;
    }) => {
      return apiClient.get<ApiResponse<any>>(`${baseUrl}/topics`, { params });
    },

    createTopic: (payload: EventFabricCreateTopicPayload) => {
      return apiClient.post<ApiResponse<EventFabricTopic>>(`${baseUrl}/topics`, payload);
    },

    updateTopicLifecycle: (topicId: string, payload: EventFabricUpdateLifecyclePayload) => {
      return apiClient.patch<ApiResponse<EventFabricTopic>>(`${baseUrl}/topics/${topicId}/lifecycle`, payload);
    },

    listDlqMessages: (params?: {
      topic?: string;
      status?: string;
      page?: number;
      page_size?: number;
    }) => {
      return apiClient.get<ApiResponse<EventFabricDlqListResult>>(
        `${baseUrl}/dlq/messages`,
        { params }
      );
    },

    replayDlqMessages: (payload: {
      message_ids: string[];
      operator_id?: string;
      notes?: string;
    }) => {
      return apiClient.post<ApiResponse<{ replayed: number }>>(
        `${baseUrl}/dlq/messages:replay`,
        payload
      );
    },

    createReplayTask: (payload: {
      topic: string;
      trace_id?: string;
      window?: { start?: string; end?: string };
      reason?: string;
      operator_id?: string;
      shadow?: boolean;
    }) => {
      return apiClient.post<ApiResponse<EventFabricReplayTaskResponse>>(
        `${baseUrl}/replay/tasks`,
        payload
      );
    },

    getReplayTask: (taskId: string) => {
      return apiClient.get<ApiResponse<EventFabricReplayTaskResponse>>(
        `${baseUrl}/replay/tasks/${taskId}`
      );
    },

    cancelReplayTask: (taskId: string, payload: { operator_id?: string }) => {
      return apiClient.post<ApiResponse<null>>(
        `${baseUrl}/replay/tasks/${taskId}/cancel`,
        payload
      );
    },

    createPipelineTask: (payload: EventFabricPipelineDebugPayload = {}) => {
      return apiClient.post<ApiResponse<EventFabricPipelineDebugResult>>(
        `${baseUrl}/pipeline/tasks`,
        payload
      );
    },

    createRetryTaskSeed: (payload: {
      topic?: string;
      subscriber_id?: string;
      reason?: string;
      immediate?: boolean;
      payload?: Record<string, any>;
    } = {}) => {
      return apiClient.post<ApiResponse<EventFabricRetrySeedResult>>(
        `${baseUrl}/retry/tasks`,
        payload
      );
    },

    getRetryTaskSeed: (deliveryId: string) => {
      return apiClient.get<ApiResponse<EventFabricRetryTaskStatus>>(
        `${baseUrl}/retry/tasks/${deliveryId}`
      );
    },

    listCronJobs: () => {
      return apiClient.get<ApiResponse<EventFabricCronJobListResult>>(
        `${baseUrl}/cron/jobs`
      );
    },

    runCronJobNow: (jobId: string) => {
      return apiClient.post<ApiResponse<EventFabricCronJob>>(
        `${baseUrl}/cron/jobs/${jobId}/run-now`,
        {}
      );
    },

    pauseCronJob: (jobId: string) => {
      return apiClient.post<ApiResponse<EventFabricCronJob>>(
        `${baseUrl}/cron/jobs/${jobId}/pause`,
        {}
      );
    },

    resumeCronJob: (jobId: string) => {
      return apiClient.post<ApiResponse<EventFabricCronJob>>(
        `${baseUrl}/cron/jobs/${jobId}/resume`,
        {}
      );
    },

    getTaskQueueStats: (params?: { subscriber_id?: string }) => {
      return apiClient.get<ApiResponse<EventFabricTaskQueueStatsResult>>(
        `${baseUrl}/task-queue/stats`,
        { params }
      );
    },
    getTaskQueueMessages: (params: { tenant_key: string; subscriber_id: string; limit?: number }) => {
      return apiClient.get<ApiResponse<EventFabricTaskQueueMessagesResult>>(
        `${baseUrl}/task-queue/messages`,
        { params }
      );
    },

    listAclBindings: (params: { topic_uuid?: string; topic_full_name?: string }) => {
      return apiClient.get<ApiResponse<{ items: EventFabricAclBinding[] }>>(
        `${baseUrl}/acl`,
        { params }
      );
    },

    upsertAclBindings: (payload: {
      topic_full_name: string;
      grants?: Array<{
        principal_type: string;
        principal_id: string;
        action: string;
        expires_at?: string;
        justification?: string;
        audit_ref?: string;
        operator_id?: string;
      }>;
      revokes?: Array<{
        principal_type: string;
        principal_id: string;
        action: string;
        operator_id?: string;
      }>;
    }) => {
      return apiClient.post<ApiResponse<{ granted: EventFabricAclBinding[]; revoked: any[] }>>(
        `${baseUrl}/acl`,
        payload
      );
    },

    getAclTopicMatrix: (params?: { namespace?: string; name?: string }) => {
      return apiClient.get<ApiResponse<EventFabricAclTopicMatrixResult>>(
        `${baseUrl}/acl/topic-matrix`,
        { params }
      );
    },

    getAclPrincipalMatrix: (params: { principal_id: string; namespace?: string; name?: string }) => {
      return apiClient.get<ApiResponse<EventFabricAclPrincipalMatrixResult>>(
        `${baseUrl}/acl/principal-matrix`,
        { params }
      );
    },
  };
};
