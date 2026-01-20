import { useApiClient } from "../index";
import type { ApiResponse } from "../types/types";

export interface NotificationRecord {
  id: string;
  title: string;
  content: string;
  type: string;
  category: string;
  isRead: boolean;
  isImportant: boolean;
  createdAt: string;
  updatedAt: string;
  userId?: string;
  relatedId?: string;
  relatedType?: string;
  actions?: any[];
  metadata?: Record<string, any>;
}

export interface NotificationListParams {
  page?: number;
  pageSize?: number;
  category?: string;
  type?: string;
  isRead?: boolean;
  isImportant?: boolean;
}

export interface NotificationListResult {
  items: NotificationRecord[];
  pagination?: {
    total: number;
    page: number;
    page_size?: number;
    pageSize?: number;
    pages?: number;
  };
}

const baseUrl = "/admin/notifications";

const buildQuery = (params?: NotificationListParams) => {
  const query = new URLSearchParams();
  if (!params) return "";
  if (params.page) query.set("page", String(params.page));
  if (params.pageSize) query.set("page_size", String(params.pageSize));
  if (params.category) query.set("category", params.category);
  if (params.type) query.set("type", params.type);
  if (typeof params.isRead === "boolean") query.set("is_read", String(params.isRead));
  if (typeof params.isImportant === "boolean") query.set("is_important", String(params.isImportant));
  const qs = query.toString();
  return qs ? `?${qs}` : "";
};

export const useNotificationService = () => {
  const apiClient = useApiClient();

  return {
    list: (params?: NotificationListParams) => {
      return apiClient.get<ApiResponse<NotificationListResult>>(
        `${baseUrl}${buildQuery(params)}`
      );
    },
    get: (uuid: string) => {
      return apiClient.get<ApiResponse<NotificationRecord>>(
        `${baseUrl}/${encodeURIComponent(uuid)}`
      );
    },
    markRead: (uuid: string) => {
      return apiClient.patch<ApiResponse<{ ok: boolean }>>(
        `${baseUrl}/${encodeURIComponent(uuid)}/read`
      );
    },
    delete: (uuid: string) => {
      return apiClient.del<ApiResponse<{ ok: boolean }>>(
        `${baseUrl}/${encodeURIComponent(uuid)}`
      );
    },
  };
};
