import { useApiClient } from "~/composables/api";

export interface KnowledgeSpacePayload {
  spaceName: string;
  departmentCode: string;
  policyTemplateVersionId: string;
  ingestionProfileKey?: string;
  indexProfileKey?: string;
  ragProfileKey?: string;
  featureFlags: string[];
  quotas: {
    cpuCores: number;
    storageGb: number;
    ingestionConcurrency: number;
  };
  requestedBy?: string;
  iamEmail?: string;
}

export interface KnowledgeSpaceUpdatePayload {
  policyTemplateVersionId?: string;
  ingestionProfileKey?: string;
  indexProfileKey?: string;
  ragProfileKey?: string;
  featureFlags?: string[];
  status?: string;
  quotas?: { cpuCores: number; storageGb: number; ingestionConcurrency: number };
  updatedBy?: string;
}

export interface KnowledgeSpaceRecord {
  spaceId: string;
  tenant_uuid: string;
  spaceName: string;
  departmentCode: string;
  status: string;
  policyTemplateVersionId: string;
  ingestionProfileKey?: string;
  indexProfileKey?: string;
  ragProfileKey?: string;
  embeddingProfileKey?: string;
  activeVectorIndexKey?: string;
  featureFlags?: string[];
  auditToken: string;
  quotas: KnowledgeSpacePayload["quotas"];
  iamStatus?: string;
  retentionExpiresAt?: string | null;
}

export interface ProfileVersionRecord {
  uuid: string;
  profileKey: string;
  version: number;
  status: string;
  displayName: string;
  config: Record<string, any>;
  rollbackFromId?: string | number;
  publishedAt?: string;
  publishedBy?: string;
  createdBy?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface CorpusCheckJobRecord {
  uuid: string;
  tenant_uuid: string;
  space_uuid: string;
  status: string;
  sample_job_uuids: string[];
  metrics: Record<string, any>;
  recommendations: any[];
  trace_id?: string;
  error_reason?: string;
  started_at?: string;
  completed_at?: string;
}

export interface StrategyValidationResult {
  ok: boolean;
  sceneKey: string;
  bundleKey: string;
  enabledChannels: string[];
  missing: Array<{ code: string; key: string; message: string; remediation: string[] }>;
  capabilities: Record<string, boolean>;
  checkedAt: string;
}

export interface KnowledgeVectorIndexRecord {
  id: number;
  space_uuid: string;
  index_key: string;
  table_name: string;
  dimensions: number;
  embedding_provider: string;
  embedding_model: string;
  embedding_profile_ref?: string;
  status: string;
  last_used_at?: string | null;
  last_error?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface VectorIndexStatus {
  spaceId: string;
  embeddingProfileKey: string;
  activeVectorIndexKey: string;
  active?: KnowledgeVectorIndexRecord | null;
  indexes: KnowledgeVectorIndexRecord[];
}

export interface RetrievalCandidateRecord {
  chunkId: string;
  score: number;
  metadata?: Record<string, any>;
  text?: string;
}

export interface RetrievalPlaygroundRecord {
  traceId: string;
  spaceId: string;
  query: string;
  profile: Record<string, any>;
  stages: Array<{ name: string; candidateCount: number; latencyMs: number; degradeReason?: string }>;
  candidates: RetrievalCandidateRecord[];
  context_pack: Record<string, any>;
}

export interface IngestionJobPayload {
  format: "pdf" | "docx" | "xlsx" | "csv" | "markdown" | "html" | "sql" | "image" | "table" | "api";
  sourceUri: string;
  ingestionProfile?: string;
  processorProfile?: string;
  ocrRequired?: boolean;
  maskingProfile?: string;
  priority?: "normal" | "high";
  // L1/L2/L3：用于审计与默认值映射（后端可存入 metrics_snapshot/config_snapshot）
  ragSceneKey?: string;
  ragBundleKey?: string;
  ragPrimary?: string;
  segmentMode?: "unit" | "heading" | "clause" | "semantic" | "table_row" | "code_block" | "conversation";
  chunkSize?: number;
  chunkOverlap?: number;
  // Anchors: 写入 chunk metadata，用于引用定位/层次索引/KG provenance
  anchorHeadingPath?: boolean;
  anchorClauseId?: boolean;
  anchorRowNumber?: boolean;
  anchorSpeaker?: boolean;
  anchorSentenceIndex?: boolean;
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
  startedAt?: string;
  completedAt?: string;
  sourceId?: string;
  sourceType?: string;
}

export interface IngestionChunkRecord {
  chunkId: string;
  kind: string;
  content: string;
  metadata?: Record<string, any>;
  confidence: number;
  masked: boolean;
}

export interface IngestionChunkListResult {
  spaceId: string;
  jobId: string;
  format?: string;
  sourceUri?: string;
  total: number;
  page: number;
  pageSize: number;
  items: IngestionChunkRecord[];
}

export interface UpdateIngestionChunkPayload {
  content: string;
  editedBy?: string;
  editReason?: string;
}

export interface DeleteIngestionJobResult {
  deleted: boolean;
  deletedChunks: number;
  deletedVectors: number;
  deletedArtifacts: boolean;
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

export interface ReleasePolicyPayload {
  matrixVersion: string;
  pilotTenants: string[];
  batches: Array<{ name: string; tenants: string[] }>;
  guardrails?: Record<string, string>;
  approvedBy?: string;
  createdBy?: string;
}

export interface ReleasePolicyRecord {
  id: number;
  matrixVersion: string;
  pilotTenants: string[];
  batches: any[];
  guardrails: Record<string, any>;
  approvedBy?: string;
  createdBy?: string;
  status: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface ReleasePublishResponse {
  releaseId: string;
  versionId: string;
  batchToken: string;
  batchIndex: number;
  tenants: string[];
}

export interface ReleasePromoteResponse {
  batchToken: string;
  batchIndex: number;
  tenants: string[];
  state: string;
  tenantCoverage: number;
}

export interface ReleaseRollbackResponse {
  status: string;
}

export interface ReleaseStatusView {
  policyId: number;
  versionId: string;
  grayState: string;
  tenantCoverage: number;
  versionDrift: number;
  alerts: string[];
  batches: Array<{
    batchToken: string;
    batchIndex: number;
    state: string;
    tenants: string[];
    alerts: string[];
    promotedAt?: string;
    completedAt?: string;
    rolledBackAt?: string;
  }>;
  recordedAt: string;
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
  const apiClient = useApiClient();

  const adminBase = "/admin/knowledge-spaces";
  const profileBase = "/admin/knowledge/profiles";
  const releaseBase = "/knowledge/release";

  const createSpace = async (
    payload: KnowledgeSpacePayload,
  ): Promise<KnowledgeSpaceRecord> => {
    const response = await apiClient.post<ApiResponse<KnowledgeSpaceRecord>>(
      adminBase,
      payload,
    );
    return response.data;
  };

  const listSpaces = async (opts?: { limit?: number; status?: string }): Promise<KnowledgeSpaceRecord[]> => {
    const params = new URLSearchParams();
    if (opts?.limit) params.set("limit", String(opts.limit));
    if (opts?.status) params.set("status", String(opts.status));
    const suffix = params.toString() ? `?${params.toString()}` : "";
    const response = await apiClient.get<ApiResponse<KnowledgeSpaceRecord[]>>(
      `${adminBase}${suffix}`,
      { useGlobalLoading: false } as any,
    );
    return response.data ?? [];
  };

  const getSpace = async (spaceId: string): Promise<KnowledgeSpaceRecord> => {
    if (!spaceId) {
      throw new Error("spaceId is required");
    }
    const response = await apiClient.get<ApiResponse<KnowledgeSpaceRecord>>(
      `${adminBase}/${encodeURIComponent(spaceId)}`,
      { useGlobalLoading: false } as any,
    );
    return response.data;
  };

    const updateSpace = async (
    spaceId: string,
    payload: KnowledgeSpaceUpdatePayload,
  ): Promise<KnowledgeSpaceRecord> => {
    const response = await apiClient.patch<ApiResponse<KnowledgeSpaceRecord>>(
      `${adminBase}/${spaceId}`,
      payload,
    );
    return response.data;
  };

const listRagProfiles = async (
    profileKey = "default",
    status = "",
  ): Promise<ProfileVersionRecord[]> => {
    const params = new URLSearchParams();
    params.set("profile_key", profileKey);
    if (status) params.set("status", status);
    const response = await apiClient.get<ApiResponse<ProfileVersionRecord[]>>(
      `${profileBase}/rag/versions?${params.toString()}`,
    );
    return response.data ?? [];
  };

  const listIndexProfiles = async (
    profileKey = "default",
    status = "",
  ): Promise<ProfileVersionRecord[]> => {
    const params = new URLSearchParams();
    params.set("profile_key", profileKey);
    if (status) params.set("status", status);
    const response = await apiClient.get<ApiResponse<ProfileVersionRecord[]>>(
      `${profileBase}/index/versions?${params.toString()}`,
    );
    return response.data ?? [];
  };

  const listIngestionProfiles = async (
    profileKey = "default",
    status = "",
  ): Promise<ProfileVersionRecord[]> => {
    const params = new URLSearchParams();
    params.set("profile_key", profileKey);
    if (status) params.set("status", status);
    const response = await apiClient.get<ApiResponse<ProfileVersionRecord[]>>(
      `${profileBase}/ingestion/versions?${params.toString()}`,
    );
    return response.data ?? [];
  };


  const validateStrategy = async (payload: { sceneKey: string; bundleKey: string }): Promise<StrategyValidationResult> => {
    const response = await apiClient.post<ApiResponse<StrategyValidationResult>>(
      `${adminBase}/strategy/validate`,
      payload,
    );
    return response.data;
  };

  const getVectorIndexStatus = async (spaceId: string): Promise<VectorIndexStatus> => {
    const response = await apiClient.get<ApiResponse<VectorIndexStatus>>(
      `${adminBase}/${spaceId}/vector-index`,
    );
    return response.data;
  };

  const activateVectorIndex = async (
    spaceId: string,
    payload: { embeddingProfileKey: string; requestedBy?: string },
  ): Promise<any> => {
    const response = await apiClient.post<ApiResponse<any>>(
      `${adminBase}/${spaceId}/vector-index/activate`,
      payload,
    );
    return response.data;
  };


  const startCorpusCheck = async (
    spaceId: string,
    requestedBy?: string,
  ): Promise<CorpusCheckJobRecord> => {
    const response = await apiClient.post<ApiResponse<CorpusCheckJobRecord>>(
      `${adminBase}/${spaceId}/corpus-check/jobs`,
      { requestedBy },
    );
    return response.data;
  };

  const getCorpusCheckJob = async (
    spaceId: string,
    jobId: string,
  ): Promise<CorpusCheckJobRecord> => {
    const response = await apiClient.get<ApiResponse<CorpusCheckJobRecord>>(
      `${adminBase}/${spaceId}/corpus-check/jobs/${jobId}`,
    );
    return response.data;
  };

  const retrievalPlayground = async (
    spaceId: string,
    payload: {
      query: string;
      ragProfileUuid?: string;
      topK?: number;
      minScore?: number;
      filters?: Record<string, string>;
    },
  ): Promise<RetrievalPlaygroundRecord> => {
    const response = await apiClient.post<ApiResponse<RetrievalPlaygroundRecord>>(
      `${adminBase}/${spaceId}/playground/retrieval`,
      payload,
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
    const response = await apiClient.post<ApiResponse<IngestionJobRecord>>(
      `${adminBase}/${spaceId}/ingestion-jobs`,
      payload,
    );
    return response.data;
  };

  const getIngestionJob = async (
    spaceId: string,
    jobId: string,
  ): Promise<IngestionJobRecord> => {
    const response = await apiClient.get<ApiResponse<IngestionJobRecord>>(
      `${adminBase}/${spaceId}/ingestion-jobs/${jobId}`,
    );
    return response.data;
  };

  const listIngestionJobs = async (
    spaceId: string,
    limit = 20,
  ): Promise<IngestionJobRecord[]> => {
    const response = await apiClient.get<ApiResponse<IngestionJobRecord[]>>(
      `${adminBase}/${spaceId}/ingestion-jobs?limit=${encodeURIComponent(String(limit))}`,
    );
    return response.data ?? [];
  };

  const listIngestionChunks = async (
    spaceId: string,
    jobId: string,
    params: { page?: number; pageSize?: number } = {},
  ): Promise<IngestionChunkListResult> => {
    const page = params.page ?? 1;
    const pageSize = params.pageSize ?? 50;
    const response = await apiClient.get<ApiResponse<IngestionChunkListResult>>(
      `${adminBase}/${spaceId}/ingestion-jobs/${jobId}/chunks?page=${encodeURIComponent(String(page))}&pageSize=${encodeURIComponent(String(pageSize))}`,
    );
    return response.data;
  };

  const updateIngestionChunk = async (
    spaceId: string,
    jobId: string,
    chunkId: string,
    payload: UpdateIngestionChunkPayload,
  ): Promise<{ updated: boolean; updatedAt?: string }> => {
    const response = await apiClient.patch<ApiResponse<{ updated: boolean; updatedAt?: string }>>(
      `${adminBase}/${spaceId}/ingestion-jobs/${jobId}/chunks/${chunkId}`,
      payload,
    );
    return response.data;
  };

  const deleteIngestionJob = async (
    spaceId: string,
    jobId: string,
  ): Promise<DeleteIngestionJobResult> => {
    const response = await apiClient.delete<ApiResponse<DeleteIngestionJobResult>>(
      `${adminBase}/${spaceId}/ingestion-jobs/${jobId}`,
    );
    return response.data;
  };

  const getIngestionPageImageBlob = async (
    spaceId: string,
    jobId: string,
    pageNumber: number,
  ): Promise<Blob> => {
    const p = Number(pageNumber);
    if (!spaceId || !jobId || !Number.isFinite(p) || p <= 0) {
      throw new Error("spaceId/jobId/pageNumber is required");
    }
    const url = `${adminBase}/${spaceId}/ingestion-jobs/${jobId}/pages/${encodeURIComponent(String(p))}/image`;
    return apiClient.request<Blob>("GET", url, undefined, {
      responseType: "blob",
      headers: { Accept: "image/*" },
      useGlobalLoading: false,
    } as any);
  };

  const listFusionStrategies = async (
    spaceId: string,
  ): Promise<FusionStrategyRecord[]> => {
    if (!spaceId) {
      return [];
    }
    const response = await apiClient.get<ApiResponse<FusionStrategyRecord[]>>(
      `${adminBase}/${spaceId}/fusion-strategies`,
    );
    return response.data ?? [];
  };

  const publishFusionStrategy = async (
    spaceId: string,
    payload: FusionStrategyPayload,
  ): Promise<FusionStrategyRecord> => {
    const response = await apiClient.post<ApiResponse<FusionStrategyRecord>>(
      `${adminBase}/${spaceId}/fusion-strategies`,
      payload,
    );
    return response.data;
  };

  const rollbackFusionStrategy = async (
    spaceId: string,
    strategyId: string,
  ): Promise<FusionStrategyRecord> => {
    const response = await apiClient.post<ApiResponse<FusionStrategyRecord>>(
      `${adminBase}/${spaceId}/fusion-strategies/${strategyId}/rollback`,
    );
    return response.data;
  };

  const listFeedbackCases = async (
    spaceId: string,
  ): Promise<FeedbackCaseRecord[]> => {
    if (!spaceId) {
      return [];
    }
    const response = await apiClient.get<ApiResponse<FeedbackCaseRecord[]>>(
      `${adminBase}/${spaceId}/feedback`,
    );
    return response.data ?? [];
  };

  const submitFeedbackCase = async (
    spaceId: string,
    payload: FeedbackCasePayload,
  ): Promise<FeedbackCaseRecord> => {
    const response = await apiClient.post<ApiResponse<FeedbackCaseRecord>>(
      `${adminBase}/${spaceId}/feedback`,
      payload,
    );
    return response.data;
  };

  const closeFeedbackCase = async (
    spaceId: string,
    caseId: string,
    payload: FeedbackCaseActionPayload,
  ): Promise<FeedbackCaseRecord> => {
    const response = await apiClient.post<ApiResponse<FeedbackCaseRecord>>(
      `${adminBase}/${spaceId}/feedback/${caseId}/close`,
      payload,
    );
    return response.data;
  };

  const escalateFeedbackCase = async (
    spaceId: string,
    caseId: string,
    payload: FeedbackCaseActionPayload,
  ): Promise<FeedbackCaseRecord> => {
    const response = await apiClient.post<ApiResponse<FeedbackCaseRecord>>(
      `${adminBase}/${spaceId}/feedback/${caseId}/escalate`,
      payload,
    );
    return response.data;
  };

  const reprocessFeedbackCase = async (
    spaceId: string,
    caseId: string,
    payload: FeedbackCaseActionPayload,
  ): Promise<FeedbackCaseRecord> => {
    const response = await apiClient.post<ApiResponse<FeedbackCaseRecord>>(
      `${adminBase}/${spaceId}/feedback/${caseId}/reprocess`,
      payload,
    );
    return response.data;
  };

  const rollbackFeedbackCase = async (
    spaceId: string,
    caseId: string,
    payload: FeedbackCaseActionPayload,
  ): Promise<FeedbackCaseRecord> => {
    const response = await apiClient.post<ApiResponse<FeedbackCaseRecord>>(
      `${adminBase}/${spaceId}/feedback/${caseId}/rollback`,
      payload,
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
    const response = await apiClient.get<ApiResponse<FeedbackExportPayload>>(
      `${adminBase}/${spaceId}/feedback/export${suffix}`,
    );
    return response.data;
  };

  const fetchStatus = async (): Promise<StatusSnapshot> => {
    return await apiClient.get<StatusSnapshot>(
      "/openapi/knowledge-spaces/status",
      { skipAuth: true, useGlobalLoading: false } as any,
    );
  };

  const listReleasePolicies = async (limit = 20): Promise<ReleasePolicyRecord[]> => {
    const params = new URLSearchParams({ limit: String(limit) });
    const response = await apiClient.get<ApiResponse<{ policies: ReleasePolicyRecord[] }>>(
      `${releaseBase}/policies?${params.toString()}`,
    );
    return response.data?.policies ?? [];
  };

  const upsertReleasePolicy = async (
    payload: ReleasePolicyPayload,
  ): Promise<{ policyId: number; status: string }> => {
    const response = await apiClient.post<ApiResponse<{ policyId: number; status: string }>>(
      `${releaseBase}/policies`,
      payload,
    );
    return response.data;
  };

  const publishRelease = async (payload: {
    policyId: string;
    versionId: string;
    requestedBy?: string;
  }): Promise<ReleasePublishResponse> => {
    const response = await apiClient.post<ApiResponse<ReleasePublishResponse>>(
      `${releaseBase}/publish`,
      payload,
    );
    return response.data;
  };

  const promoteRelease = async (payload: {
    policyId: string;
    versionId: string;
    batchToken: string;
    alerts?: string[];
    requestedBy?: string;
  }): Promise<ReleasePromoteResponse> => {
    const response = await apiClient.post<ApiResponse<ReleasePromoteResponse>>(
      `${releaseBase}/promote`,
      payload,
    );
    return response.data;
  };

  const rollbackRelease = async (payload: {
    policyId: string;
    versionId: string;
    reason?: string;
    requestedBy?: string;
  }): Promise<ReleaseRollbackResponse> => {
    const response = await apiClient.post<ApiResponse<ReleaseRollbackResponse>>(
      `${releaseBase}/rollback`,
      payload,
    );
    return response.data;
  };

  const getReleaseStatus = async (
    policyId: string,
    versionId?: string,
  ): Promise<ReleaseStatusView | null> => {
    const params = new URLSearchParams({ policyId });
    if (versionId) {
      params.set("versionId", versionId);
    }
    const response = await apiClient.get<ApiResponse<{ status: ReleaseStatusView | null }>>(
      `${releaseBase}/status?${params.toString()}`,
    );
    return response.data?.status ?? null;
  };

  return {
    createSpace,
    updateSpace,
    listSpaces,
    getSpace,
    fetchStatus,
    listRagProfiles,
    listIndexProfiles,
    validateStrategy,
    getVectorIndexStatus,
    activateVectorIndex,
    listIngestionProfiles,
    startCorpusCheck,
    getCorpusCheckJob,
    retrievalPlayground,
    triggerIngestion,
    getIngestionJob,
    listIngestionJobs,
    listIngestionChunks,
    deleteIngestionJob,
    updateIngestionChunk,
    getIngestionPageImageBlob,
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
    listReleasePolicies,
    upsertReleasePolicy,
    publishRelease,
    promoteRelease,
    rollbackRelease,
    getReleaseStatus,
  };
};
