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

const adminBase = "/admin/skills";
const tenantBase = "/tenant/skills";

export const useSkillsService = () => {
  const api = useApiClient();

  return {
    list: (params?: Record<string, string | number | undefined>) =>
      api.get<ApiResponse<SkillListResult>>(adminBase, { params }),

    listCatalog: () => api.get<ApiResponse<{ items: Array<Record<string, unknown>> }>>(`${adminBase}/catalog`),

    importSkill: (payload: SkillImportPayload) =>
      api.post<ApiResponse<SkillRecord>>(`${adminBase}/import`, payload),

    publish: (skillId: string, version: string, approvalNote?: string) =>
      api.post<ApiResponse<SkillRecord>>(`${adminBase}/${encodeURIComponent(skillId)}/publish`, {
        version,
        approval_note: approvalNote,
      }),

    rollback: (skillId: string, targetVersion: string, reason: string) =>
      api.post<ApiResponse<SkillRecord>>(`${adminBase}/${encodeURIComponent(skillId)}/rollback`, {
        target_version: targetVersion,
        reason,
      }),

    invoke: (payload: SkillInvokePayload) =>
      api.post<ApiResponse<Record<string, unknown>>>(`${tenantBase}/invoke`, payload),
  };
};
