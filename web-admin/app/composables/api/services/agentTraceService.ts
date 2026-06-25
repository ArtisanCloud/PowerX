import { useApiClient } from "../index";
import type {
  AgentTraceQuery,
  AgentTraceReport,
  AgentTraceTimelineResult,
  AgentRunListResult,
  AgentSessionListResult,
} from "../types/agentTrace";

const base = "/admin/agent-traces";

const unwrap = <T>(response: any): T => {
  if (response && typeof response === "object" && "data" in response) {
    return response.data as T;
  }
  return response as T;
};

const queryParams = (query: AgentTraceQuery, extra: Record<string, string> = {}) => ({
  tenant_uuid: query.tenant_uuid,
  session_id: query.session_id,
  run_id: query.run_id || undefined,
  trace_id: query.trace_id || undefined,
  ...extra,
});

export const useAgentTraceService = () => {
  const api = useApiClient();

  const getMessage = async (query: AgentTraceQuery) => {
    const response = await api.get(`${base}/messages/${encodeURIComponent(query.message_id)}`, {
      params: queryParams(query),
    });
    return unwrap<Record<string, unknown>>(response);
  };

  const getTimeline = async (query: AgentTraceQuery) => {
    const response = await api.get(`${base}/messages/${encodeURIComponent(query.message_id)}/timeline`, {
      params: queryParams(query),
    });
    return unwrap<AgentTraceTimelineResult>(response);
  };

  const getReport = async (query: AgentTraceQuery) => {
    const response = await api.get(`${base}/messages/${encodeURIComponent(query.message_id)}/report`, {
      params: queryParams(query),
    });
    return unwrap<AgentTraceReport>(response);
  };

  const listRuns = async (params: { tenant_uuid: string; session_id?: string; status?: string; offset?: number; limit?: number }) => {
    const response = await api.get(`${base}/runs`, {
      params: {
        tenant_uuid: params.tenant_uuid,
        session_id: params.session_id || undefined,
        status: params.status || undefined,
        offset: params.offset || 0,
        limit: params.limit || 50,
      },
    });
    return unwrap<AgentRunListResult>(response);
  };

  const listSessions = async (params: { tenant_uuid: string; status?: string; offset?: number; limit?: number }) => {
    const response = await api.get(`${base}/sessions`, {
      params: {
        tenant_uuid: params.tenant_uuid,
        status: params.status || undefined,
        offset: params.offset || 0,
        limit: params.limit || 50,
      },
    });
    return unwrap<AgentSessionListResult>(response);
  };

  const downloadUrl = (query: AgentTraceQuery, format: "json" | "markdown") => {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(queryParams(query, format === "markdown" ? { format: "markdown", download: "md" } : { download: "json" }))) {
      if (value) params.set(key, String(value));
    }
    return `/api/v1${base}/messages/${encodeURIComponent(query.message_id)}/report?${params.toString()}`;
  };

  const sessionDownloadUrl = (query: AgentTraceQuery, format: "json" | "markdown") => {
    const params = new URLSearchParams();
    const extra = format === "markdown" ? { format: "markdown", download: "md" } : { download: "json" };
    for (const [key, value] of Object.entries({
      tenant_uuid: query.tenant_uuid,
      ...extra,
    })) {
      if (value) params.set(key, String(value));
    }
    return `/api/v1${base}/sessions/${encodeURIComponent(query.session_id)}/report?${params.toString()}`;
  };

  return {
    getMessage,
    getTimeline,
    getReport,
    listRuns,
    listSessions,
    downloadUrl,
    sessionDownloadUrl,
  };
};
