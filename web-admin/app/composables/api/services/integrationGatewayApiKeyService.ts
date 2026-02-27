import { useApiClient } from "../index";
import type { ApiResponse } from "../types/types";

export interface IntegrationGatewayApiKeyPermission {
  scope: string;
  action: string;
  resource_type: string;
  resource_pattern: string;
  plugin_id?: string;
  effect?: string;
}

export interface IntegrationGatewayPermissionCatalogItem {
  id: number;
  module: string;
  resource: string;
  action: string;
  description?: string;
  status: string;
  meta?: Record<string, any>;
}

export interface IntegrationGatewayApiKeyRecord {
  key_id: string;
  tenant_uuid: string;
  profile_id: number;
  name: string;
  description?: string;
  key_prefix: string;
  status: string;
  expires_at?: string;
  last_used_at?: string;
  created_at: string;
  updated_at: string;
  permissions?: IntegrationGatewayApiKeyPermission[];
}

export interface IntegrationGatewayApiKeyProfile {
  id: number;
  tenant_uuid: string;
  key: string;
  name: string;
  status: number;
}

export interface CreateIntegrationGatewayApiKeyProfilePayload {
  tenant_uuid: string;
  key?: string;
  name?: string;
}

export interface UpdateIntegrationGatewayApiKeyProfilePayload {
  tenant_uuid: string;
  name?: string;
  status?: number;
}

export interface IntegrationGatewayApiKeyListResult {
  items: IntegrationGatewayApiKeyRecord[];
  pagination?: {
    total: number;
    page: number;
    page_size?: number;
    pageSize?: number;
    pages?: number;
  };
}

export interface CreateIntegrationGatewayApiKeyPayload {
  tenant_uuid: string;
  profile_id: number;
  name: string;
  description?: string;
  expires_at?: string;
}

export interface RotateIntegrationGatewayApiKeyPayload {
  tenant_uuid: string;
  name?: string;
  description?: string;
  expires_at?: string;
}

export interface RevokeIntegrationGatewayApiKeyPayload {
  tenant_uuid: string;
}

export interface CreateIntegrationGatewayApiKeyResult {
  api_key: IntegrationGatewayApiKeyRecord;
  plain_key: string;
}

export interface ProfilePermissionsResult {
  profile_id: number;
  permission_ids: number[];
  permission_rows: IntegrationGatewayPermissionCatalogItem[];
}

export interface RotateIntegrationGatewayApiKeyResult {
  api_key: IntegrationGatewayApiKeyRecord;
  plain_key: string;
  rotated: string;
}

const baseUrl = "/admin/integration";

const buildQuery = (params: Record<string, string | number | undefined>) => {
  const query = new URLSearchParams();
  Object.entries(params).forEach(([key, value]) => {
    if (value === undefined || value === null || value === "") return;
    query.set(key, String(value));
  });
  const qs = query.toString();
  return qs ? `?${qs}` : "";
};

export const useIntegrationGatewayApiKeyService = () => {
  const apiClient = useApiClient();

  return {
    listApiKeyProfiles: (tenantUUID: string) => {
      return apiClient.get<ApiResponse<{ items: IntegrationGatewayApiKeyProfile[] }>>(
        `${baseUrl}/api-key-profiles${buildQuery({ tenant_uuid: tenantUUID })}`
      );
    },
    createApiKeyProfile: (payload: CreateIntegrationGatewayApiKeyProfilePayload) => {
      return apiClient.post<ApiResponse<IntegrationGatewayApiKeyProfile>>(
        `${baseUrl}/api-key-profiles`,
        payload
      );
    },
    updateApiKeyProfile: (profileID: number, payload: UpdateIntegrationGatewayApiKeyProfilePayload) => {
      return apiClient.patch<ApiResponse<IntegrationGatewayApiKeyProfile>>(
        `${baseUrl}/api-key-profiles/${encodeURIComponent(String(profileID))}`,
        payload
      );
    },
    listPermissionCatalog: () => {
      return apiClient.get<ApiResponse<{ items: IntegrationGatewayPermissionCatalogItem[] }>>(
        `${baseUrl}/permissions/catalog`
      );
    },
    getProfilePermissions: (profileID: number) => {
      return apiClient.get<ApiResponse<ProfilePermissionsResult>>(
        `${baseUrl}/api-key-profiles/${encodeURIComponent(String(profileID))}/permissions`
      );
    },
    setProfilePermissions: (profileID: number, permissionIDs: number[]) => {
      return apiClient.put<ApiResponse<{ profile_id: number; permission_ids: number[]; added: number[]; removed: number[]; synced_keys?: number; synced_perms?: number }>>(
        `${baseUrl}/api-key-profiles/${encodeURIComponent(String(profileID))}/permissions`,
        { permission_ids: permissionIDs }
      );
    },
    createApiKey: (payload: CreateIntegrationGatewayApiKeyPayload) => {
      return apiClient.post<ApiResponse<CreateIntegrationGatewayApiKeyResult>>(
        `${baseUrl}/api-keys`,
        payload
      );
    },
    listApiKeys: (params: { tenant_uuid: string; page?: number; page_size?: number }) => {
      return apiClient.get<ApiResponse<IntegrationGatewayApiKeyListResult>>(
        `${baseUrl}/api-keys${buildQuery(params)}`
      );
    },
    getApiKey: (keyID: string) => {
      return apiClient.get<ApiResponse<IntegrationGatewayApiKeyRecord>>(
        `${baseUrl}/api-keys/${encodeURIComponent(keyID)}`
      );
    },
    revokeApiKey: (keyID: string, payload: RevokeIntegrationGatewayApiKeyPayload) => {
      return apiClient.post<ApiResponse<{ status: string; key_id: string }>>(
        `${baseUrl}/api-keys/${encodeURIComponent(keyID)}/revoke`,
        payload
      );
    },
    deleteApiKey: (keyID: string) => {
      return apiClient.delete<ApiResponse<{ status: string; key_id: string }>>(
        `${baseUrl}/api-keys/${encodeURIComponent(keyID)}`
      );
    },
    rotateApiKey: (keyID: string, payload: RotateIntegrationGatewayApiKeyPayload) => {
      return apiClient.post<ApiResponse<RotateIntegrationGatewayApiKeyResult>>(
        `${baseUrl}/api-keys/${encodeURIComponent(keyID)}/rotate`,
        payload
      );
    },
  };
};
