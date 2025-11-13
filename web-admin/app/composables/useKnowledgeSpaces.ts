export interface KnowledgeSpacePayload {
  tenantId: string;
  spaceName: string;
  departmentCode: string;
  policyTemplateVersionId: string;
  featureFlags: string[];
  quotas: {
    cpuCores: number;
    storageGb: number;
    ingestionConcurrency: number;
  };
  requestedBy?: string;
  iamEmail?: string;
}

export interface KnowledgeSpaceRecord {
  spaceId: string;
  spaceName: string;
  departmentCode: string;
  status: string;
  policyTemplateVersionId: string;
  auditToken: string;
  quotas: KnowledgeSpacePayload["quotas"];
}

interface ApiResponse<T> {
  code: number;
  message: string;
  data: T;
}

interface StatusSnapshot {
  pendingIam: number;
  active: number;
  retired: number;
}

export const useKnowledgeSpaces = () => {
  const config = useRuntimeConfig();
  const baseURL = config.public?.apiBase || "/api";

  const createSpace = async (
    payload: KnowledgeSpacePayload,
  ): Promise<KnowledgeSpaceRecord> => {
    const response = await $fetch<ApiResponse<KnowledgeSpaceRecord>>(
      `${baseURL}/admin/knowledge-spaces`,
      {
        method: "POST",
        body: payload,
      },
    );
    return response.data;
  };

  const fetchStatus = async (): Promise<StatusSnapshot> => {
    return await $fetch<StatusSnapshot>(
      `${baseURL}/openapi/knowledge-spaces/status`,
    );
  };

  return {
    createSpace,
    fetchStatus,
  };
};
