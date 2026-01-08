import { useApiClient } from "../index";
import type { ApiResponse } from "../types/types";

export interface EventFabricTopic {
  id: string;
  uuid: string;
  full_topic: string;
  namespace: string;
  name: string;
  lifecycle: string;
  payload_format: string;
  max_retry: number;
  ack_timeout_sec: number;
  versioning_mode: string;
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

export const useEventFabricService = () => {
  const apiClient = useApiClient();
  const baseUrl = "/event-fabric";

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
      page?: number;
      page_size?: number;
    }) => {
      return apiClient.get<ApiResponse<any>>(`${baseUrl}/topics`, { params });
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
  };
};

