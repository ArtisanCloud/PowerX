import { useApiClient } from "../index";
import { ApiEndpoints } from "../config";
import type { ApiResponse } from "../types/types";

export interface QuotaEntryResponse {
  providerId: string;
  limit: number;
  usage: number;
  status: string;
  anomaly?: Record<string, any>;
  enforcement?: {
    history?: any[];
    [key: string]: any;
  };
}

export interface QuotaSnapshotResponse {
  tenantId: string;
  quotas: QuotaEntryResponse[];
}

export interface EnforceActionPayload {
  env?: string;
  tenantId: string;
  providerId?: string;
  action: string;
  reason?: string;
  ticketId?: string;
  requestedBy?: string;
}

export class CostQuotaService {
  static async getQuotaSnapshot(params: {
    tenantId: string;
    env?: string;
  }): Promise<QuotaSnapshotResponse | undefined> {
    const { get } = useApiClient();
    const response = await get<ApiResponse<QuotaSnapshotResponse>>(
      ApiEndpoints.ADMIN_AGENTS.COST_QUOTAS,
      {
        params,
      }
    );
    return response.data;
  }

  static async enforceAction(payload: EnforceActionPayload): Promise<{
    ok: boolean;
  }> {
    const { post } = useApiClient();
    const response = await post<ApiResponse<{ ok: boolean }>>(
      ApiEndpoints.ADMIN_AGENTS.COST_ENFORCE,
      payload
    );
    return response.data || { ok: false };
  }

  static async reportUsage(payload: {
    tenantId: string;
    providerId?: string;
    env?: string;
    events: Array<{ costUsd: number; tokens?: number; timestamp?: string }>;
  }) {
    const { post } = useApiClient();
    return post(ApiEndpoints.ADMIN_AGENTS.COST_USAGE_REPORT, payload);
  }
}
