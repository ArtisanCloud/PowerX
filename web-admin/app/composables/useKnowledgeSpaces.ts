export interface KnowledgeSpacePayload {
  tenantUuid: string;
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
  tenantUuid?: string;
  spaceName: string;
  departmentCode: string;
  status: string;
  policyTemplateVersionId: string;
  auditToken: string;
  quotas: KnowledgeSpacePayload["quotas"];
}

export interface IngestionJobPayload {
  format: "pdf" | "docx" | "xlsx" | "csv" | "markdown" | "html" | "sql" | "image" | "table" | "api";
  sourceUri: string;
  ingestionProfile?: string;
  processorProfile?: string;
  ocrRequired?: boolean;
  maskingProfile?: string;
  priority?: "normal" | "high";
}

export interface IngestionJobRecord {
  jobId: string;
  status: string;
  retryCount: number;
  errorCode?: string;
  reason?: string;
  chunkTotal: number;
  chunkCoveragePct: number;
  embeddingSuccessPct: number;
  maskingCoveragePct: number;
}

export interface FusionStrategyPayload {
  label: string;
  bm25Weight: number;
  vectorWeight: number;
  graphConstraint: string;
  rerankerModel: string;
  conflictPolicy?: "block" | "queue" | "allow_with_flag";
  requestedBy?: string;
}

export interface FusionStrategyRecord {
  strategyId: string;
  spaceId?: string;
  label: string;
  bm25Weight: number;
  vectorWeight: number;
  graphConstraint: string;
  rerankerModel: string;
  conflictPolicy: string;
  deploymentState: string;
  degraded?: boolean;
  degradeReasons?: string[];
  publishedAt?: string;
}

export interface FeedbackCasePayload {
  severity: "low" | "medium" | "high" | "critical";
  issueType: "accuracy" | "freshness" | "compliance";
  linkedChunks: string[];
  notes?: string;
  reportedBy?: string;
  toolTraceRef?: string;
}

export interface FeedbackCaseRecord {
  caseId: string;
  status: string;
  severity: string;
  issueType: string;
  linkedChunks: string[];
  reportedBy: string;
  slaDueAt?: string;
  qualityScore: number;
  reprocessJobId?: string;
  traceId?: string;
  toolTraceRef?: string;
  escalatedAt?: string;
  closedAt?: string;
  resolutionNotes?: string;
  createdAt: string;
  updatedAt: string;
}

export interface FeedbackCaseActionPayload {
  requestedBy?: string;
  resolutionNotes?: string;
  reason?: string;
}

export interface FeedbackExportPayload {
  cases: FeedbackCaseRecord[];
  audits: any[];
  meta: Record<string, any>;
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

  const listFusionStrategies = async (
    spaceId: string,
  ): Promise<FusionStrategyRecord[]> => {
    if (!spaceId) {
      return [];
    }
    const response = await $fetch<ApiResponse<FusionStrategyRecord[]>>(
      `${adminPath(`/${spaceId}/fusion-strategies`)}`,
      {
        method: "GET",
      },
    );
    return response.data ?? [];
  };

  const publishFusionStrategy = async (
    spaceId: string,
    payload: FusionStrategyPayload,
  ): Promise<FusionStrategyRecord> => {
    const response = await $fetch<ApiResponse<FusionStrategyRecord>>(
      `${adminPath(`/${spaceId}/fusion-strategies`)}`,
      {
        method: "POST",
        body: payload,
      },
    );
    return response.data;
  };

  const rollbackFusionStrategy = async (
    spaceId: string,
    strategyId: string,
  ): Promise<FusionStrategyRecord> => {
    const response = await $fetch<ApiResponse<FusionStrategyRecord>>(
      `${adminPath(`/${spaceId}/fusion-strategies/${strategyId}/rollback`)}`,
      {
        method: "POST",
      },
    );
    return response.data;
  };

  const listFeedbackCases = async (
    spaceId: string,
  ): Promise<FeedbackCaseRecord[]> => {
    if (!spaceId) {
      return [];
    }
    const response = await $fetch<ApiResponse<FeedbackCaseRecord[]>>(
      `${adminPath(`/${spaceId}/feedback`)}`,
      {
        method: "GET",
      },
    );
    return response.data ?? [];
  };

  const submitFeedbackCase = async (
    spaceId: string,
    payload: FeedbackCasePayload,
  ): Promise<FeedbackCaseRecord> => {
    const response = await $fetch<ApiResponse<FeedbackCaseRecord>>(
      `${adminPath(`/${spaceId}/feedback`)}`,
      {
        method: "POST",
        body: payload,
      },
    );
    return response.data;
  };

  const closeFeedbackCase = async (
    spaceId: string,
    caseId: string,
    payload: FeedbackCaseActionPayload,
  ): Promise<FeedbackCaseRecord> => {
    const response = await $fetch<ApiResponse<FeedbackCaseRecord>>(
      `${adminPath(`/${spaceId}/feedback/${caseId}/close`)}`,
      {
        method: "POST",
        body: payload,
      },
    );
    return response.data;
  };

  const escalateFeedbackCase = async (
    spaceId: string,
    caseId: string,
    payload: FeedbackCaseActionPayload,
  ): Promise<FeedbackCaseRecord> => {
    const response = await $fetch<ApiResponse<FeedbackCaseRecord>>(
      `${adminPath(`/${spaceId}/feedback/${caseId}/escalate`)}`,
      {
        method: "POST",
        body: payload,
      },
    );
    return response.data;
  };

  const reprocessFeedbackCase = async (
    spaceId: string,
    caseId: string,
    payload: FeedbackCaseActionPayload,
  ): Promise<FeedbackCaseRecord> => {
    const response = await $fetch<ApiResponse<FeedbackCaseRecord>>(
      `${adminPath(`/${spaceId}/feedback/${caseId}/reprocess`)}`,
      {
        method: "POST",
        body: payload,
      },
    );
    return response.data;
  };

  const rollbackFeedbackCase = async (
    spaceId: string,
    caseId: string,
    payload: FeedbackCaseActionPayload,
  ): Promise<FeedbackCaseRecord> => {
    const response = await $fetch<ApiResponse<FeedbackCaseRecord>>(
      `${adminPath(`/${spaceId}/feedback/${caseId}/rollback`)}`,
      {
        method: "POST",
        body: payload,
      },
    );
    return response.data;
  };

  const exportFeedbackCases = async (
    spaceId: string,
    query?: { status?: string; severity?: string; limit?: number },
  ): Promise<FeedbackExportPayload> => {
    const params = new URLSearchParams();
    if (query?.status) {
      params.set("status", query.status);
    }
    if (query?.severity) {
      params.set("severity", query.severity);
    }
    if (query?.limit) {
      params.set("limit", String(query.limit));
    }
    const suffix = params.toString() ? `?${params.toString()}` : "";
    const response = await $fetch<ApiResponse<FeedbackExportPayload>>(
      `${adminPath(`/${spaceId}/feedback/export`)}${suffix}`,
      {
        method: "GET",
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
    listFusionStrategies,
    publishFusionStrategy,
    rollbackFusionStrategy,
    listFeedbackCases,
    submitFeedbackCase,
    closeFeedbackCase,
    escalateFeedbackCase,
    reprocessFeedbackCase,
    rollbackFeedbackCase,
    exportFeedbackCases,
  };
};
