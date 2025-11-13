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
  tenantId?: string;
  spaceName: string;
  departmentCode: string;
  status: string;
  policyTemplateVersionId: string;
  auditToken: string;
  quotas: KnowledgeSpacePayload["quotas"];
}

export interface IngestionJobPayload {
  sourceType: "pdf" | "markdown" | "table" | "api";
  sourceUri: string;
  maskingProfile?: string;
  priority?: "normal" | "high";
}

export interface IngestionJobRecord {
  jobId: string;
  status: string;
  chunkTotal: number;
  chunkCoveragePct: number;
  embeddingSuccessPct: number;
  maskingCoveragePct: number;
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

  const adminPath = (path: string) => `${baseURL}/admin/knowledge-spaces${path}`;

  const createSpace = async (
    payload: KnowledgeSpacePayload,
  ): Promise<KnowledgeSpaceRecord> => {
    const response = await $fetch<ApiResponse<KnowledgeSpaceRecord>>(
      adminPath(""),
      {
        method: "POST",
        body: payload,
      },
    );
    return response.data;
  };

  const triggerIngestion = async (
    spaceId: string,
    payload: IngestionJobPayload,
  ): Promise<IngestionJobRecord> => {
    if (!spaceId) {
      throw new Error("spaceId is required");
    }
    const response = await $fetch<ApiResponse<IngestionJobRecord>>(
      `${adminPath(`/${spaceId}/ingestion-jobs`)}`,
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
    triggerIngestion,
  };
};
