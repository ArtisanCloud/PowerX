export interface QARetrievalPlanRequest {
  tenantUuid: string;
  intent: string;
  domainTags?: string[];
  sessionId?: string;
  latencyBudgetMs?: number;
}

export interface QACandidateSpace {
  spaceId: string;
  spaceName: string;
  strategy: string;
  citationCoverage: number;
  degradeReason?: string;
}

export interface QAToolMetadata {
  toolId: string;
  name: string;
  category: string;
  endpoint: string;
}

export interface QATelemetry {
  traceId: string;
  recordedAt: string;
}

export interface QARetrievalPlanResponse {
  tenantUuid: string;
  intent: string;
  domainTags: string[];
  candidateSpaces: QACandidateSpace[];
  tooling: QAToolMetadata[];
  telemetry: QATelemetry;
  degradeCount: number;
  sessionId?: string;
  latencyBudgetMs?: number;
}

export interface QAMemorySnapshotRequest {
  tenantUuid: string;
  sessionId: string;
  updates?: QACitationSummary[];
}

export interface QACitationSummary {
  chunkId: string;
  spaceId: string;
  status: string;
  citations: string[];
  sourceType?: string;
  confidence?: number;
  deltaReason?: string;
}

export interface QAMemorySnapshotResponse {
  tenantUuid: string;
  sessionId: string;
  citations: QACitationSummary[];
}

interface ApiResponse<T> {
  code: number;
  message: string;
  data: T;
}

export const createQaBridgeClient = () => {
  const config = useRuntimeConfig();
  const baseURL = config.public?.apiBase || "/api";
  const qaPath = (suffix: string) =>
    `${baseURL}/openapi/knowledge-spaces/qa${suffix}`;

  const plan = async (
    payload: QARetrievalPlanRequest,
  ): Promise<QARetrievalPlanResponse> => {
    const response = await $fetch<ApiResponse<QARetrievalPlanResponse>>(
      qaPath("/retrieval-plan"),
      {
        method: "POST",
        body: payload,
      },
    );
    return response.data;
  };

  const snapshot = async (
    payload: QAMemorySnapshotRequest,
  ): Promise<QAMemorySnapshotResponse> => {
    const response = await $fetch<ApiResponse<QAMemorySnapshotResponse>>(
      qaPath("/memory-snapshot"),
      {
        method: "POST",
        body: payload,
      },
    );
    return response.data;
  };

  return { plan, snapshot };
};
