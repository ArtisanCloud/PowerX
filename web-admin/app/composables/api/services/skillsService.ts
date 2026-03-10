import { useApiClient } from "../index";
import type { ApiResponse } from "../types/types";

export interface SkillRecord {
  skill_id: string;
  version: string;
  source: "builtin" | "plugin" | "third_party";
  status: "draft" | "published" | "deprecated" | "disabled";
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

    listCatalog: () => api.get<ApiResponse<{ items: Array<Record<string, unknown>> }>>(`${adminBase}/catalog`),

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
  };
};
