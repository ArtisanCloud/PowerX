import { useApiClient } from "../index";
import type { ApiResponse } from "../types/types";

export interface AgentTeamRecord {
  id: number;
  uuid?: string;
  tenant_uuid: string;
  parent_agent_id: number;
  team_name: string;
  dispatch_mode: "serial" | "parallel" | "mixed";
  default_failure_policy: "fail-fast" | "continue" | "retry-once";
  status: "active" | "disabled";
  created_by?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface AgentTeamMemberRecord {
  id: number;
  team_id: number;
  tenant_uuid: string;
  child_agent_id: number;
  role: "retriever" | "executor" | "reviewer";
  priority: number;
  enabled: boolean;
}

const base = "/admin/agents/teams";

export const useAgentTeamService = () => {
  const api = useApiClient();
  const unwrap = <T>(resp: any): T =>
    resp && typeof resp === "object" && "data" in resp
      ? (resp as any).data
      : resp;

  return {
    createTeam: async (payload: {
      parent_agent_id: number;
      team_name: string;
      dispatch_mode?: string;
      default_failure_policy?: string;
    }) => {
      const resp = await api.post<ApiResponse<AgentTeamRecord>>(base, payload);
      return unwrap<AgentTeamRecord>(resp);
    },

    listTeams: async (parentAgentId?: number, includeDisabled = false) => {
      const params: Record<string, any> = {
        include_disabled: includeDisabled,
      };
      if (typeof parentAgentId === "number" && parentAgentId > 0) {
        params.parent_agent_id = parentAgentId;
      }
      const resp = await api.get<ApiResponse<{ items: AgentTeamRecord[]; total: number }>>(base, {
        params,
      });
      return unwrap<{ items: AgentTeamRecord[]; total: number }>(resp);
    },

    updateTeamStatus: async (teamId: number, status: "active" | "disabled") => {
      const resp = await api.patch<ApiResponse<{ team_id: number; status: string }>>(
        `${base}/${teamId}/status`,
        { status }
      );
      return unwrap<{ team_id: number; status: string }>(resp);
    },

    updateTeam: async (
      teamId: number,
      payload: {
        parent_agent_id?: number;
        team_name?: string;
        dispatch_mode?: "serial" | "parallel" | "mixed";
        default_failure_policy?: "fail-fast" | "continue" | "retry-once";
      }
    ) => {
      const resp = await api.patch<ApiResponse<AgentTeamRecord>>(`${base}/${teamId}`, payload);
      return unwrap<AgentTeamRecord>(resp);
    },

    deleteTeam: async (teamId: number) => {
      const resp = await api.delete<ApiResponse<{ team_id: number; deleted: boolean }>>(`${base}/${teamId}`);
      return unwrap<{ team_id: number; deleted: boolean }>(resp);
    },

    upsertMember: async (
      teamId: number,
      payload: {
        child_agent_id: number;
        role?: string;
        priority?: number;
        enabled?: boolean;
      }
    ) => {
      const resp = await api.put<ApiResponse<AgentTeamMemberRecord>>(
        `${base}/${teamId}/members`,
        payload
      );
      return unwrap<AgentTeamMemberRecord>(resp);
    },

    listMembers: async (teamId: number) => {
      const resp = await api.get<ApiResponse<{ items: AgentTeamMemberRecord[]; total: number }>>(
        `${base}/${teamId}/members`
      );
      return unwrap<{ items: AgentTeamMemberRecord[]; total: number }>(resp);
    },

    deleteMember: async (teamId: number, childAgentId: number) => {
      const resp = await api.delete<ApiResponse<{ deleted: boolean }>>(
        `${base}/${teamId}/members/${childAgentId}`
      );
      return unwrap<{ deleted: boolean }>(resp);
    },
  };
};
