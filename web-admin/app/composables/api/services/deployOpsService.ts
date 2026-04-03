import { useApiClient } from "../index";

export interface DeployReleaseRecord {
  id: number | string;
  environment: string;
  backend_version: string;
  web_admin_version: string;
  action: "release" | "rollback" | string;
  status: "pending" | "running" | "success" | "failed" | string;
  operator: string;
  trace_id?: string;
  started_at?: string;
  ended_at?: string;
  error_message?: string;
}

export interface DeployHealthSummary {
  status: string;
  summary: string;
}

export interface DeployReleaseListResult {
  items: DeployReleaseRecord[];
  pagination?: {
    total: number;
    page: number;
    page_size: number;
  };
}

export interface TriggerReleasePayload {
  environment: string;
  backend_version: string;
  web_admin_version: string;
}

export interface TriggerRollbackPayload {
  environment: string;
  target_version: string;
}

const unwrap = <T>(payload: unknown): T => {
  if (payload && typeof payload === "object" && "data" in (payload as any)) {
    return (payload as any).data as T;
  }
  return payload as T;
};

const adminBase = "/admin/deploy";

export const useDeployOpsService = () => {
  const api = useApiClient();

  return {
    async listReleases(params?: {
      environment?: string;
      page?: number;
      pageSize?: number;
    }): Promise<DeployReleaseListResult> {
      const response = await api.get(`${adminBase}/releases`, {
        params: {
          environment: params?.environment,
          page: params?.page,
          page_size: params?.pageSize,
        },
      });
      const data = unwrap<{ items?: DeployReleaseRecord[]; pagination?: any }>(response) || {};
      return {
        items: Array.isArray(data.items) ? data.items : [],
        pagination: data.pagination,
      };
    },

    async triggerRelease(payload: TriggerReleasePayload, options?: {
      mode?: "docker" | "systemd";
      approvalTickets?: number;
    }): Promise<DeployReleaseRecord> {
      const response = await api.post(`${adminBase}/releases`, payload, {
        params: {
          mode: options?.mode,
          approval_tickets: options?.approvalTickets,
        },
      });
      const data = unwrap<{ release: DeployReleaseRecord }>(response);
      return data.release;
    },

    async triggerRollback(payload: TriggerRollbackPayload, options?: {
      mode?: "docker" | "systemd";
      approvalTickets?: number;
    }): Promise<DeployReleaseRecord> {
      const response = await api.post(`${adminBase}/rollback`, payload, {
        params: {
          mode: options?.mode,
          approval_tickets: options?.approvalTickets,
        },
      });
      const data = unwrap<{ release: DeployReleaseRecord }>(response);
      return data.release;
    },

    async getHealth(): Promise<DeployHealthSummary> {
      const response = await api.get(`${adminBase}/health`);
      return unwrap<DeployHealthSummary>(response);
    },
  };
};
