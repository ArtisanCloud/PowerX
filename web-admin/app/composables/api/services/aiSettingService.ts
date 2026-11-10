import { useApiClient } from "../index";
import { ApiEndpoints } from "../config";
import { reactive, toRefs } from "vue";
import type { ApiResponse } from "../types/types";
import type { PowerModel } from "../types";

export interface Provider {
  id: string;
  name: string;
  apps?: { id: string; name: string }[];
  auth?: {
    scheme?: string;
    fields?: string[];
    defaults?: Record<string, string>;
    modes?: Array<{
      id: string;
      label?: string;
      scheme?: string;
      fields?: string[];
      defaults?: Record<string, string>;
    }>;
  };
}

interface ProviderCatalogResponse {
  env: string;
  modality: string;
  providers: Provider[];
}

export interface VisionModelUsage {
  status: 'supported' | 'unsupported' | 'unknown';
  reason_code?: string;
  source: 'provider_api' | 'official_docs' | 'unknown';
  source_url?: string;
  checked_at?: string;
}

export interface OnlineCatalogModel {
  operations?: Record<string, ImageOperation>;
  parameter_profile?: ModelParameterProfile;
  id: string;
  label: string;
  modalities: string[];
  match: "local" | "provider" | "manual" | "unmatched";
  manual_modalities: string[];
  reported_modalities?: string[];
  manual_confirmed?: boolean;
  review_required?: boolean;
  input_modalities?: string[];
  output_modalities?: string[];
  vision?: VisionModelUsage;
}

export interface ImageOperation {
  driver: string;
  parameters: Record<string, { type: string; enum?: Array<string | number>; default?: unknown }>;
}

export interface TenantModelCatalog {
  models: string[];
  catalog_source: 'online' | 'instance' | 'local' | 'unknown';
  source?: string;
  updated_at?: string;
  items?: OnlineCatalogModel[];
}

export interface ModelParameterProfile {
  version: number;
  provider: string;
  model: string;
  source: 'official_docs' | 'provider_api' | 'unknown';
  source_url?: string;
  checked_at?: string;
  defaults: Record<string, unknown>;
  sampling: 'supported' | 'reasoning_none' | 'unsupported' | 'unknown';
  reasoning_efforts?: string[];
  token_parameter?: string;
  supported_parameters?: string[];
  supported_parameters_known?: boolean;
  max_output_tokens?: number;
}

export interface OnlineModelCatalog {
  provider: string;
  env: string;
  app: string;
  protocol: string;
  source: string;
  updated_at: string;
  revision: string;
  models: OnlineCatalogModel[];
  corrected_count?: number;
}

export interface OnlineModelSyncRequest {
  env: string;
  provider: string;
  app: string;
  modality: string;
  action: "refresh" | "assign" | "unassign";
  revision?: string;
  models?: string[];
  confirm_unknown?: boolean;
}

export interface ConnectionVerification {
  provider: string;
  probe: "models" | "key_info" | "ollama_tags";
  endpoint_reachable: boolean;
  api_key_verified: boolean;
  authentication: "api_key" | "not_required";
  model_access_verified: false;
  generation_submitted: false;
}

export interface ConnectionTestResult {
  ok: boolean;
  verification?: ConnectionVerification;
  message?: string;
}

export interface VisionTrialContext {
  env: string;
  provider: string;
  model: string;
  app: string;
  connection?: { baseURL: string; apiKey?: string; authMode: string; organization: string; region: string; secretId: string; secretKey?: string; azureDeployment: string };
  params: { maxTokens: number; jsonMode: boolean; reasoningEffort?: string; temperature?: number; topP?: number };
}
export interface VisionTrialResult {
  type: 'analysis';
  provider: string;
  model: string;
  text: string;
  trace_id: string;
  elapsed_ms: number;
  env: string;
  usage?: Record<string, unknown>;
}
export type VisionTrialImage = { type: 'image_url'; url: string } | { type: 'image_base64'; content: string };

export interface AgentProfile extends PowerModel {
  modality: string;
  provider: string;
  model: string;
  label: string;
  defaults: {
    [key: string]: any;
    maxTokens: number;
    stream: boolean;
    temperature: number;
    topP: number;
  };
  capCache: Record<string, any>;
  tags: string[];
}

export interface AgentCredential extends PowerModel {
  name: string;
  provider: string;
  authScheme: string;
  has_credentials?: boolean;
  data: {
    azure_deployment: string;
    base_url: string;
    organization: string;
    region: string;
  };
}

export interface SaveSettingsPayload {
  env?: string;
  modality: string;
  llm?: {
    provider: string;
    app?: string;
    model: string;
    apiKey?: string;
    baseURL?: string;
    organization?: string;
    region?: string;
    azureDeployment?: string;
    temperature?: number;
    reasoningEffort?: string;
    maxTokens?: number;
    topP?: number;
    stream?: boolean;
  };
  vlm?: {
    provider: string; app?: string; model: string; apiKey?: string; baseURL?: string;
    organization?: string; region?: string; reuseCredential?: boolean;
    maxTokens?: number; reasoningEffort?: string; temperature?: number; topP?: number; jsonMode?: boolean;
  };
  image?: {
    provider: string;
    app?: string;
    model: string;
    apiKey?: string;
    baseURL?: string;
    organization?: string;
    region?: string;
    azureDeployment?: string;
    size?: string;
    quality?: string;
    format?: string;
    promptHint?: string;
  };
  embedding?: {
    provider: string;
    app?: string;
    model: string;
    apiKey?: string;
    baseURL?: string;
    organization?: string;
    region?: string;
    azureDeployment?: string;
    dimensions?: number;
    truncate?: string;
    batch?: number;
  };
  audio_tts?: {
    provider: string;
    app?: string;
    model: string;
    apiKey?: string;
    baseURL?: string;
    organization?: string;
    region?: string;
    azureDeployment?: string;
    voice?: string;
    speed?: number;
    format?: string;
    quality?: string;
  };
  audio_asr?: {
    provider: string;
    app?: string;
    model: string;
    apiKey?: string;
    baseURL?: string;
    organization?: string;
    region?: string;
    azureDeployment?: string;
    language?: string;
    responseFormat?: string;
    temperature?: number;
    prompt?: string;
  };
  video?: {
    provider: string;
    app?: string;
    model: string;
    apiKey?: string;
    baseURL?: string;
    organization?: string;
    region?: string;
    azureDeployment?: string;
    resolution?: string;
    fps?: number;
    maxDurationSec?: number;
    promptHint?: string;
  };
  model3d?: {
    provider: string;
    app?: string;
    model: string;
    apiKey?: string;
    baseURL?: string;
    organization?: string;
    region?: string;
    azureDeployment?: string;
    outputFormat?: string;
    promptHint?: string;
  };
  rerank?: {
    provider: string;
    app?: string;
    model: string;
    apiKey?: string;
    baseURL?: string;
    organization?: string;
    region?: string;
    azureDeployment?: string;
    topK?: number;
    returnDocuments?: boolean;
    maxChunksPerDoc?: number;
  };
}

export interface SkillsSourcePolicy {
  allowlist: string[];
  effective_source: "tenant" | "default" | string;
  updated_at?: string;
}

export interface ContextOptimizerConfig {
  enabled: boolean;
  max_prompt_tokens: number;
  reserved_completion_tokens: number;
  recent_messages: number;
  retrieval_top_k: number;
  cache_mode: "auto" | "force_on" | "force_off" | string;
  summary_refresh_interval_sec: number;
  debug_trace_enabled: boolean;
  planner_enabled: boolean;
  planner_candidate_top_k: number;
  planner_prompt_slim_mode: "compact" | "verbose" | string;
  planner_decision_cache_enabled: boolean;
  planner_decision_cache_ttl_sec: number;
  planner_quota_workflow: number;
  planner_quota_skill: number;
  planner_quota_tooling: number;
  planner_quota_llm: number;
}

export interface ContextOptimizerActiveResponse {
  env: string;
  scope: "tenant" | "system" | string;
  active: {
    source: "tenant" | "system" | "yaml_default" | string;
    version: number;
    scope: "tenant" | "system" | string;
    config: ContextOptimizerConfig;
    updated_at?: string;
  };
}

export interface ContextOptimizerVersionItem {
  id: number;
  uuid: string;
  env: string;
  scope: string;
  tenant_uuid?: string;
  version: number;
  status: "draft" | "published" | "archived" | string;
  config: ContextOptimizerConfig;
  change_reason?: string;
  updated_at: string;
  published_at?: string;
}

export class AISettingService {
  static async getSkillsSourcePolicy(): Promise<SkillsSourcePolicy> {
    const { get } = useApiClient();
    const response = await get<ApiResponse<SkillsSourcePolicy>>(
      ApiEndpoints.ADMIN_AGENTS.SKILLS_SOURCE_POLICY
    );
    const data: any = response.data || {};
    return {
      allowlist: Array.isArray(data.allowlist) ? data.allowlist : [],
      effective_source: String(data.effective_source || "default"),
      updated_at: data.updated_at,
    };
  }

  static async setSkillsSourcePolicy(
    allowlist: string[]
  ): Promise<SkillsSourcePolicy> {
    const { put } = useApiClient();
    const response = await put<ApiResponse<SkillsSourcePolicy>>(
      ApiEndpoints.ADMIN_AGENTS.SKILLS_SOURCE_POLICY,
      { allowlist }
    );
    const data: any = response.data || {};
    return {
      allowlist: Array.isArray(data.allowlist) ? data.allowlist : [],
      effective_source: String(data.effective_source || "tenant"),
      updated_at: data.updated_at,
    };
  }

  static async getContextOptimizerActive(params: {
    env: string;
    scope?: "tenant" | "system";
  }): Promise<ContextOptimizerActiveResponse> {
    const { get } = useApiClient();
    const search = new URLSearchParams();
    search.append("env", params.env);
    if (params.scope) search.append("scope", params.scope);
    const response = await get<ApiResponse<ContextOptimizerActiveResponse>>(
      `${ApiEndpoints.ADMIN_AGENTS.CONTEXT_OPTIMIZER_ACTIVE}?${search.toString()}`
    );
    return response.data;
  }

  static async listContextOptimizerVersions(params: {
    env: string;
    scope?: "tenant" | "system";
    limit?: number;
  }): Promise<{
    env: string;
    scope: string;
    versions: ContextOptimizerVersionItem[];
  }> {
    const { get } = useApiClient();
    const search = new URLSearchParams();
    search.append("env", params.env);
    if (params.scope) search.append("scope", params.scope);
    if (params.limit) search.append("limit", String(params.limit));
    const response = await get<ApiResponse<{
      env: string;
      scope: string;
      versions: ContextOptimizerVersionItem[];
    }>>(`${ApiEndpoints.ADMIN_AGENTS.CONTEXT_OPTIMIZER_VERSIONS}?${search.toString()}`);
    return response.data || { env: params.env, scope: params.scope || "tenant", versions: [] };
  }

  static async saveContextOptimizerDraft(payload: {
    env: string;
    scope?: "tenant" | "system";
    config: ContextOptimizerConfig;
    change_reason?: string;
  }): Promise<{ draft: ContextOptimizerVersionItem }> {
    const { post } = useApiClient();
    const response = await post<ApiResponse<{ draft: ContextOptimizerVersionItem }>>(
      ApiEndpoints.ADMIN_AGENTS.CONTEXT_OPTIMIZER_DRAFTS,
      payload
    );
    return response.data;
  }

  static async publishContextOptimizer(payload: {
    env: string;
    scope?: "tenant" | "system";
    version: number;
    change_reason?: string;
  }): Promise<{ published: ContextOptimizerVersionItem }> {
    const { post } = useApiClient();
    const response = await post<ApiResponse<{ published: ContextOptimizerVersionItem }>>(
      ApiEndpoints.ADMIN_AGENTS.CONTEXT_OPTIMIZER_PUBLISH,
      payload
    );
    return response.data;
  }

  static async rollbackContextOptimizer(payload: {
    env: string;
    scope?: "tenant" | "system";
    target_version: number;
    change_reason?: string;
  }): Promise<{ rolled_back: ContextOptimizerVersionItem }> {
    const { post } = useApiClient();
    const response = await post<ApiResponse<{ rolled_back: ContextOptimizerVersionItem }>>(
      ApiEndpoints.ADMIN_AGENTS.CONTEXT_OPTIMIZER_ROLLBACK,
      payload
    );
    return response.data;
  }

  /**
   * 获取可用的供应商列表
   */
  static async getProviders(modality?: string, env?: string): Promise<Provider[]> {
    const { get } = useApiClient();
    const params = new URLSearchParams();
    if (modality) params.append("modality", modality);
    if (env) params.append("env", env);
    const url = params.toString()
      ? `${ApiEndpoints.ADMIN_AGENTS.PROVIDERS}?${params.toString()}`
      : ApiEndpoints.ADMIN_AGENTS.PROVIDERS;
    const response = await get<ApiResponse<ProviderCatalogResponse>>(
      url
    );
    const providers = response.data?.providers;
    if (!Array.isArray(providers) || providers.some(provider =>
      !provider || typeof provider.id !== "string" || !provider.id.trim() ||
      typeof provider.name !== "string" || !provider.name.trim()
    )) {
      throw new Error("AI_PROVIDER_CATALOG_INVALID");
    }
    return providers;
  }

  /**
   * 获取可用的模型列表
   */
  static async getModels(
    provider?: string,
    modality?: string,
    env?: string,
    app?: string,
    baseURL?: string
  ): Promise<string[]> {
    return (await this.getModelCatalog(provider, modality, env, app, baseURL)).models;
  }

  static async getModelCatalog(provider?: string, modality?: string, env?: string, app?: string, baseURL?: string): Promise<TenantModelCatalog> {
    const { get } = useApiClient();
    const params = new URLSearchParams();
    if (provider) params.append("provider", provider);
    if (modality) params.append("modality", modality);
    if (env) params.append("env", env);
    if (app) params.append("app", app);
    // 私有实例发现使用表单地址，尚未保存的地址也可读取；不在 URL 中携带密钥。
    if (provider?.trim().toLowerCase() === "ollama" && baseURL?.trim()) params.append("base_url", baseURL.trim());

    const url = params.toString()
      ? `${ApiEndpoints.ADMIN_AGENTS.MODELS}?${params.toString()}`
      : ApiEndpoints.ADMIN_AGENTS.MODELS;

    const response = await get<ApiResponse<string[]>>(url, { useGlobalLoading: false });
    const data: any = response.data as any;
    if (Array.isArray(data)) return { models: data, catalog_source: 'unknown' };
    if (Array.isArray(data?.models)) return { ...data, catalog_source: data.catalog_source || 'unknown' };
    if (Array.isArray(data?.items)) return { models: data.items, catalog_source: 'unknown' };
    return { models: [], catalog_source: 'unknown' };
  }

  static async syncOnlineModels(payload: OnlineModelSyncRequest): Promise<OnlineModelCatalog> {
    const { post } = useApiClient();
    const response = await post<ApiResponse<OnlineModelCatalog>>(ApiEndpoints.ADMIN_AGENTS.ONLINE_MODELS_SYNC, payload, { useGlobalLoading: false });
    const result = response.data;
    if (!result || !result.revision || !Array.isArray(result.models)) throw new Error("AI_MODEL_DISCOVERY_RESPONSE_INVALID");
    return result;
  }

  /**
   * 保存AI设置
   */
  static async saveSettings(
    payload: SaveSettingsPayload
  ): Promise<{ ok: boolean }> {
    const { post } = useApiClient();
    const response = await post<ApiResponse<{ ok: boolean }>>(
      ApiEndpoints.ADMIN_AGENTS.SETTINGS_SAVE,
      payload
    );
    return response.data || { ok: false };
  }

  /**
   * 测试连接
   */
  static async testConnection(payload: SaveSettingsPayload): Promise<ConnectionTestResult> {
    const { post } = useApiClient();
    const response = await post<ApiResponse<ConnectionTestResult>>(
      ApiEndpoints.ADMIN_AGENTS.TEST_CONNECTION,
      payload,
      { useGlobalLoading: false }
    );
    return response.data || { ok: false };
  }

  /**
   * 测试快速调用
   */
  static async testQuickCall(payload: SaveSettingsPayload): Promise<any> {
    const { post } = useApiClient();
    const response = await post<ApiResponse<any>>(
      ApiEndpoints.ADMIN_AGENTS.TEST_CALL,
      payload,
      { useGlobalLoading: false }
    );
    return response.data;
  }

  /** 显式执行一次视觉试跑；由 Core 配置控制推理期限，不被前端通用 30 秒截断。 */
  static async trialVision(context: VisionTrialContext, image: VisionTrialImage, prompt: string, signal: AbortSignal): Promise<VisionTrialResult> {
    const { post } = useApiClient();
    const response = await post<ApiResponse<{ ok: boolean; generation_submitted: boolean; result: VisionTrialResult }>>(
      ApiEndpoints.ADMIN_AGENTS.TEST_CALL,
      { env: context.env, modality: 'vlm', execute: true,
        ...(context.connection ? { useFormConnection: true } : {}),
        vlm: { provider: context.provider, app: context.app, model: context.model, ...(!context.connection ? { reuseCredential: true } : {}), ...context.connection, ...context.params },
        inputs: [{ type: 'text', content: prompt }, image] },
      { signal, timeout: 0, retry: 0, useGlobalLoading: false, useGlobalError: false },
    );
    const data = response.data;
    if (data?.generation_submitted === false) throw Object.assign(new Error('AI_VISION_TRIAL_NOT_SUPPORTED'), { errorCode: 'AI_VISION_TRIAL_NOT_SUPPORTED' });
    if (!data?.ok || data.generation_submitted !== true || data.result?.type !== 'analysis' || !data.result.text?.trim() || !data.result.trace_id) {
      throw Object.assign(new Error('AI_OUTPUT_INVALID'), { errorCode: 'AI_OUTPUT_INVALID' });
    }
    return data.result;
  }

  /**
   * 获取配置文件列表
   */
  static async getProfiles(
    env?: string,
    modalities?: string[]
  ): Promise<{
    env: string;
    profiles: AgentProfile[];
  }> {
    const { get } = useApiClient();
    const params = new URLSearchParams();
    if (env) params.append("env", env);
    if (modalities?.length) {
      params.append("modalities", modalities.join(","));
    }
    const url = params.toString()
      ? `${ApiEndpoints.ADMIN_AGENTS.PROFILES}?${params.toString()}`
      : ApiEndpoints.ADMIN_AGENTS.PROFILES;
    const response = await get<
      ApiResponse<{ env: string; profiles: AgentProfile[] }>
    >(url);
    return response.data || { env: "default", profiles: [] };
  }

  /**
   * 获取凭证列表
   */
  static async getCredentials(env?: string, options: { useGlobalLoading?: boolean } = {}): Promise<{
    env: string;
    credentials: AgentCredential[];
  }> {
    const { get } = useApiClient();
    const params = new URLSearchParams();
    if (env) params.append("env", env);
    const url = params.toString()
      ? `${ApiEndpoints.ADMIN_AGENTS.CREDENTIALS}?${params.toString()}`
      : ApiEndpoints.ADMIN_AGENTS.CREDENTIALS;
    const response = await get<
      ApiResponse<{ env: string; credentials: AgentCredential[] }>
    >(url, options);
    return response.data || { env: "default", credentials: [] };
  }

  /**
   * 获取当前激活的配置
   */
  static async getActiveProfile(
    env: string = "default",
    modality: string = "llm"
  ): Promise<
    ApiResponse<{
      env: string;
      modality: string;
      profile: AgentProfile;
    }>
  > {
    const { get } = useApiClient();
    const params = new URLSearchParams();
    params.append("env", env);
    params.append("modality", modality);

    const url = `${ApiEndpoints.ADMIN_AGENTS.SETTINGS_ACTIVE}?${params.toString()}`;

    return await get<
      ApiResponse<{
        env: string;
        modality: string;
        profile: AgentProfile;
      }>
    >(url);
  }

  /**
   * 设置当前激活的配置
   */
  static async setActiveProfile(payload: {
    env: string;
    modality: string;
    provider: string;
    model: string;
  }): Promise<ApiResponse<{ ok: boolean }>> {
    const { post } = useApiClient();
    return await post<ApiResponse<{ ok: boolean }>>(
      ApiEndpoints.ADMIN_AGENTS.SETTINGS_ACTIVE,
      payload
    );
  }
}
