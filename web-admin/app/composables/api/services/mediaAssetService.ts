import { useApiClient } from "../index";

export type MediaAssetBusinessStatus =
  | "draft"
  | "under_review"
  | "published"
  | "archived"
  | string;

export type MediaAssetDriver = "local" | "s3" | string;

export interface MediaAssetAdminView {
  uuid: string;
  tenant_uuid: string;
  name: string;
  description?: string;
  driver: MediaAssetDriver;
  folder?: string;
  objectKey: string;
  externalUrl?: string;
  sizeBytes?: number | null;
  mimeType?: string;
  ownerSubjectType?: string;
  ownerSubjectId?: string;
  tags?: string[];
  businessStatus: MediaAssetBusinessStatus;
  downloadUrl?: string;
  downloadExpiredAt?: string;
  createdAt: string;
  updatedAt: string;
  deleted?: boolean;
  metadata?: Record<string, any>;
}

export interface MediaAssetListParams {
  page?: number;
  pageSize?: number;
  keyword?: string;
  driver?: string;
  ownerSubjectType?: string;
  ownerSubjectId?: string;
  businessStatus?: string[];
  tags?: string[];
  includeDeleted?: boolean;
  onlyDeleted?: boolean;
  uuid?: string[];
}

export interface MediaAssetPagination {
  total: number;
  page: number;
  page_size?: number;
  pageSize?: number;
  pages?: number;
}

export interface MediaAssetListResult {
  items: MediaAssetAdminView[];
  pagination: MediaAssetPagination;
}

export type MediaAssetUploadMethod =
  | "direct_upload"
  | "external_link"
  | "presign_upload";

export interface CreateMediaAssetPayload {
  name: string;
  description?: string;
  driver?: string;
  bucket?: string;
  baseUrl?: string;
  folder?: string;
  ownerSubjectType?: string;
  ownerSubjectId?: string;
  tags?: string[];
  uploadMethod?: MediaAssetUploadMethod;
  externalUrl?: string;
  objectKey?: string;
  sizeBytes?: number;
  mimeType?: string;
  metadata?: Record<string, any>;
}

export interface UpdateMediaAssetPayload {
  name?: string;
  description?: string;
  businessStatus?: string;
  tags?: string[];
  metadata?: Record<string, any>;
}

export interface PresignMediaAssetPayload {
  action: "upload" | "download";
  method?: "GET" | "PUT" | "POST";
  expiresInSeconds?: number;
  expires_in?: number;
  filename?: string;
  content_type?: string;
  headers?: Record<string, string>;
  metadata?: Record<string, string>;
}

export interface PresignMediaAssetResult {
  url: string;
  method: string;
  expiresInSeconds: number;
  expiresAt?: string;
  headers?: Record<string, string>;
  objectKey?: string;
  storageKey?: string;
  storage_key?: string;
}

const baseUrl = "/admin/media/assets";

export const useMediaAssetService = () => {
  const apiClient = useApiClient();

  return {
    listAssets: async (params: MediaAssetListParams = {}): Promise<MediaAssetListResult> => {
      const resp = await apiClient.get(baseUrl, { params });
      const { items, pagination } = apiClient.unwrapList<MediaAssetAdminView>(resp);
      return {
        items,
        pagination: {
          ...(pagination || {}),
          pageSize: pagination?.pageSize || pagination?.page_size,
        },
      };
    },

    createAsset: async (payload: CreateMediaAssetPayload): Promise<MediaAssetAdminView> => {
      const resp = await apiClient.post(baseUrl, payload);
      return apiClient.unwrap<MediaAssetAdminView>(resp);
    },

    getAsset: async (uuid: string): Promise<MediaAssetAdminView> => {
      const id = uuid?.trim();
      const resp = await apiClient.get(`${baseUrl}/${encodeURIComponent(id)}`);
      return apiClient.unwrap<MediaAssetAdminView>(resp);
    },

    updateAsset: async (
      uuid: string,
      payload: UpdateMediaAssetPayload
    ): Promise<MediaAssetAdminView> => {
      const id = uuid?.trim();
      const resp = await apiClient.patch(`${baseUrl}/${encodeURIComponent(id)}`, payload);
      return apiClient.unwrap<MediaAssetAdminView>(resp);
    },

    deleteAsset: async (uuid: string): Promise<void> => {
      const id = uuid?.trim();
      await apiClient.delete(`${baseUrl}/${encodeURIComponent(id)}`);
    },

    presign: async (uuid: string, payload: PresignMediaAssetPayload): Promise<PresignMediaAssetResult> => {
      const id = uuid?.trim();
      const resp = await apiClient.post(`${baseUrl}/${encodeURIComponent(id)}/presign`, payload);
      return apiClient.unwrap<PresignMediaAssetResult>(resp);
    },

    buildResourcePath: (uuid: string, disposition: "inline" | "attachment" = "inline") => {
      const id = uuid?.trim();
      return `${baseUrl}/${encodeURIComponent(id)}/resource?disposition=${encodeURIComponent(disposition)}`;
    },

    getResourceBlob: async (
      uuid: string,
      disposition: "inline" | "attachment" = "inline"
    ): Promise<Blob> => {
      const url = `${baseUrl}/${encodeURIComponent(uuid.trim())}/resource?disposition=${encodeURIComponent(disposition)}`;
      return apiClient.request<Blob>("GET", url, undefined, {
        responseType: "blob",
        headers: { Accept: "*/*" },
        useGlobalLoading: false,
      } as any);
    },
  };
};

