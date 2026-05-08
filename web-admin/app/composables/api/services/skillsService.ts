import { useApiClient } from "../index";
import type { ApiResponse } from "../types/types";

export interface SkillRecord {
  skill_id: string;
  version: string;
  source: "builtin" | "plugin" | "third_party";
  status: "draft" | "published" | "deprecated" | "disabled";
  import_type?: string;
  bundle_uri?: string;
  checksum?: string;
  signature?: string;
  source_url?: string;
  source_ref?: string;
  is_latest_published?: boolean;
  updated_at?: string;
}

export interface SkillListResult {
  page: number;
  page_size: number;
  total: number;
  items: SkillRecord[];
}

export interface SkillImportPayload {
  skill_id: string;
  version: string;
  source: "plugin" | "third_party";
  bundle_uri: string;
  checksum: string;
  signature?: string;
  source_url?: string;
  source_ref?: string;
}

export interface SkillInvokePayload {
  skill_id: string;
  version?: string;
  payload: Record<string, unknown>;
  context?: Record<string, unknown>;
}

export interface SkillInstallTaskPayload {
  provider?: "github" | "gitlab" | "gitee" | "bitbucket" | "generic_git";
  repo?: string;
  repo_url?: string;
  path: string;
  ref?: string;
  method?: "auto" | "download" | "git";
  source?: "third_party" | "plugin";
  skill_id?: string;
  version?: string;
  auto_import?: boolean;
}

export interface SkillInstallTaskRecord {
  task_id: string;
  provider: string;
  repo: string;
  repo_url?: string;
  path: string;
  ref: string;
  method: string;
  source: string;
  skill_id: string;
  version: string;
  install_path?: string;
  status: "pending" | "running" | "success" | "failed";
  stdout_log?: string;
  stderr_log?: string;
  error_summary?: string;
  requested_by?: string;
  started_at?: string;
  finished_at?: string;
  created_at?: string;
  updated_at?: string;
}

export interface SkillInstallTaskListResult {
  page: number;
  page_size: number;
  total: number;
  items: SkillInstallTaskRecord[];
}

export interface SkillAuditRecord {
  audit_id: string;
  action: string;
  skill_id: string;
  version: string;
  operator: string;
  tenant_scope: string;
  reason?: string;
  result: string;
  trace_id?: string;
  source?: string;
  error_summary?: string;
  created_at?: string;
}

export interface SkillCatalogItem {
  catalog_skill_id: string;
  skill_id: string;
  recommended_version: string;
  risk_level: string;
  category?: string;
  summary?: string;
  active?: boolean;
  maintainer?: string;
  official_release_note?: string;
}

export interface SkillTraceRecord {
  trace_id: string;
  tenant_uuid: string;
  skill_id: string;
  version?: string;
  entrypoint?: string;
  protocol_used?: string;
  invoke_path?: string;
  status?: string;
  latency_ms?: number;
  error_code?: string;
  error_summary?: string;
  request_payload_digest?: string;
  response_payload_digest?: string;
  capability_id?: string;
  plan_id?: string;
  node_id?: string;
  team_id?: string;
  handoff_task_id?: string;
  handoff_trace_id?: string;
  node_status?: string;
  retry_trace?: string;
  fallback_used?: boolean;
  authorization_check_pass?: boolean;
  created_at?: string;
}

export interface UpsertSkillCatalogPayload {
  catalog_skill_id: string;
  skill_id: string;
  recommended_version: string;
  risk_level: string;
  category: string;
  summary?: string;
  active?: boolean;
  maintainer?: string;
  official_release_note?: string;
  bundle_uri?: string;
  checksum?: string;
}

const adminBase = "/admin/skills";
const tenantBase = "/tenant/skills";

function mapSkillsError(error: any): Error {
  const responseData = error?.data ?? error?.response?._data ?? {};
  const rawMessage = String(
    responseData?.error ||
      responseData?.message ||
      error?.message ||
      "Skills 操作失败"
  );
  const normalized = rawMessage.toLowerCase();

  if (normalized.includes("checksum mismatch")) {
    return new Error("校验和不匹配，仅支持 sha256 校验值");
  }
  if (normalized.includes("checksum is required")) {
    return new Error("缺少 checksum，无法导入或发布");
  }
  if (normalized.includes("signature is required")) {
    return new Error("当前策略要求签名，请补充 signature");
  }
  if (normalized.includes("remote repository online pull is disabled")) {
    return new Error("仅允许上传后的 bundle_uri，禁止远程仓库在线拉取");
  }
  if (normalized.includes("skill not found")) {
    return new Error("Skill 不存在或版本不存在");
  }
  return new Error(rawMessage);
}

export const useSkillsService = () => {
  const api = useApiClient();
  const unwrap = <T>(resp: any): T =>
    resp && typeof resp === "object" && "data" in resp
      ? (resp as any).data
      : resp;

  return {
    list: (params?: Record<string, string | number | undefined>) =>
      api.get<ApiResponse<SkillListResult>>(adminBase, { params }),

    listCatalog: () => api.get<ApiResponse<{ items: SkillCatalogItem[] }>>(`${adminBase}/catalog`),

    upsertCatalog: async (payload: UpsertSkillCatalogPayload) => {
      try {
        const resp = await api.post<ApiResponse<SkillCatalogItem>>(
          `${adminBase}/catalog`,
          payload
        );
        return unwrap<SkillCatalogItem>(resp);
      } catch (error: any) {
        throw mapSkillsError(error);
      }
    },

    setCatalogActive: async (catalogSkillId: string, active: boolean) => {
      try {
        const resp = await api.patch<ApiResponse<{ catalog_skill_id: string; active: boolean }>>(
          `${adminBase}/catalog/${encodeURIComponent(catalogSkillId)}/active`,
          { active }
        );
        return unwrap<{ catalog_skill_id: string; active: boolean }>(resp);
      } catch (error: any) {
        throw mapSkillsError(error);
      }
    },

    importSkill: async (payload: SkillImportPayload) => {
      try {
        const resp = await api.post<ApiResponse<SkillRecord>>(
          `${adminBase}/import`,
          payload
        );
        return unwrap<SkillRecord>(resp);
      } catch (error: any) {
        throw mapSkillsError(error);
      }
    },

    publish: async (skillId: string, version: string, approvalNote?: string) => {
      try {
        const resp = await api.post<ApiResponse<SkillRecord>>(
          `${adminBase}/${encodeURIComponent(skillId)}/publish`,
          {
            version,
            approval_note: approvalNote,
          }
        );
        return unwrap<SkillRecord>(resp);
      } catch (error: any) {
        throw mapSkillsError(error);
      }
    },

    rollback: async (skillId: string, targetVersion: string, reason: string) => {
      try {
        const resp = await api.post<ApiResponse<SkillRecord>>(
          `${adminBase}/${encodeURIComponent(skillId)}/rollback`,
          {
            target_version: targetVersion,
            reason,
          }
        );
        return unwrap<SkillRecord>(resp);
      } catch (error: any) {
        throw mapSkillsError(error);
      }
    },

    invoke: (payload: SkillInvokePayload) =>
      api.post<ApiResponse<Record<string, unknown>>>(`${tenantBase}/invoke`, payload),

    listAudits: async (skillId: string, limit = 20) => {
      const resp = await api.get<ApiResponse<{ items: SkillAuditRecord[]; total: number }>>(
        `${adminBase}/audits`,
        { params: { skill_id: skillId, limit } }
      );
      return unwrap<{ items: SkillAuditRecord[]; total: number }>(resp);
    },

    getTrace: async (traceId: string, tenantUUID?: string) => {
      const resp = await api.get<ApiResponse<Record<string, unknown>>>(
        `${adminBase}/traces/${encodeURIComponent(traceId)}`,
        { params: { tenant_uuid: tenantUUID } }
      );
      return unwrap<Record<string, unknown>>(resp);
    },

    listTraces: async (params?: {
      tenant_uuid?: string;
      skill_id?: string;
      version?: string;
      plan_id?: string;
      node_id?: string;
      team_id?: string;
      handoff_task_id?: string;
      handoff_trace_id?: string;
      node_status?: string;
      limit?: number;
      offset?: number;
    }) => {
      const resp = await api.get<ApiResponse<{ items: SkillTraceRecord[]; total: number; limit: number; offset: number }>>(
        `${adminBase}/traces`,
        { params }
      );
      return unwrap<{ items: SkillTraceRecord[]; total: number; limit: number; offset: number }>(resp);
    },

    createInstallTask: async (payload: SkillInstallTaskPayload) => {
      try {
        const resp = await api.post<ApiResponse<SkillInstallTaskRecord>>(
          `${adminBase}/install-tasks`,
          payload
        );
        return unwrap<SkillInstallTaskRecord>(resp);
      } catch (error: any) {
        throw mapSkillsError(error);
      }
    },

    listInstallTasks: async (params?: {
      status?: string;
      provider?: string;
      repo?: string;
      skill_id?: string;
      page?: number;
      page_size?: number;
    }) => {
      try {
        const resp = await api.get<ApiResponse<SkillInstallTaskListResult>>(
          `${adminBase}/install-tasks`,
          { params }
        );
        return unwrap<SkillInstallTaskListResult>(resp);
      } catch (error: any) {
        throw mapSkillsError(error);
      }
    },

    getInstallTask: async (taskId: string) => {
      try {
        const resp = await api.get<ApiResponse<SkillInstallTaskRecord>>(
          `${adminBase}/install-tasks/${encodeURIComponent(taskId)}`
        );
        return unwrap<SkillInstallTaskRecord>(resp);
      } catch (error: any) {
        throw mapSkillsError(error);
      }
    },
  };
};
