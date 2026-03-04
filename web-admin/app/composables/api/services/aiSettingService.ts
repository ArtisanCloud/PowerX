import { useApiClient } from "../index";
import { ApiEndpoints } from "../config";
import { reactive, toRefs } from "vue";
import type { ApiResponse } from "../types/types";
import type { PowerModel } from "../types";

export interface Provider {
  ID: string;
  Name: string;
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

export interface AgentProfile extends PowerModel {
  modality: string;
  provider: string;
  model: string;
  label: string;
  defaults: {
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
  data: {
    api_key: string;
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
    maxTokens?: number;
    topP?: number;
    stream?: boolean;
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

export class AISettingService {
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
    const response = await get<ApiResponse<Provider[]>>(
      url
    );
    return response.data.providers || [];
  }

  /**
   * 获取可用的模型列表
   */
  static async getModels(
    provider?: string,
    modality?: string,
    env?: string,
    app?: string
  ): Promise<string[]> {
    const { get } = useApiClient();
    const params = new URLSearchParams();
    if (provider) params.append("provider", provider);
    if (modality) params.append("modality", modality);
    if (env) params.append("env", env);
    if (app) params.append("app", app);

    const url = params.toString()
      ? `${ApiEndpoints.ADMIN_AGENTS.MODELS}?${params.toString()}`
      : ApiEndpoints.ADMIN_AGENTS.MODELS;

    const response = await get<ApiResponse<string[]>>(url);
    const data: any = response.data as any;
    if (Array.isArray(data)) return data;
    if (Array.isArray(data?.models)) return data.models;
    if (Array.isArray(data?.items)) return data.items;
    return [];
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
  static async testConnection(payload: SaveSettingsPayload): Promise<any> {
    const { post } = useApiClient();
    const response = await post<ApiResponse<any>>(
      ApiEndpoints.ADMIN_AGENTS.TEST_CONNECTION,
      payload
    );
    return response.data;
  }

  /**
   * 测试快速调用
   */
  static async testQuickCall(payload: SaveSettingsPayload): Promise<any> {
    const { post } = useApiClient();
    const response = await post<ApiResponse<any>>(
      ApiEndpoints.ADMIN_AGENTS.TEST_CALL,
      payload
    );
    return response.data;
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
  static async getCredentials(env?: string): Promise<{
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
    >(url);
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
