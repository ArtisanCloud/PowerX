import { useApiClient } from "../index";
import { ApiEndpoints } from "../config";
import { reactive, toRefs } from "vue";
import type { ApiResponse } from "../types/types";
import type { PowerModel } from "../types";

export interface Provider {
  ID: string;
  Name: string;
  auth?: {
    scheme?: string;
    fields?: string[];
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
  rerank?: {
    provider: string;
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

export interface TestConnectionPayload {
  provider: string;
  credentials: {
    api_key: string;
    base_url: string;
    organization?: string;
    region?: string;
  };
}

export interface TestQuickCallPayload {
  provider: string;
  model: string;
  credentials: {
    api_key: string;
    base_url: string;
    organization?: string;
    region?: string;
  };
  message?: string;
}

export class AISettingService {
  /**
   * 获取可用的供应商列表
   */
  static async getProviders(): Promise<Provider[]> {
    const { get } = useApiClient();
    const response = await get<ApiResponse<Provider[]>>(
      ApiEndpoints.ADMIN_AGENTS.PROVIDERS
    );
    return response.data.providers || [];
  }

  /**
   * 获取可用的模型列表
   */
  static async getModels(
    provider?: string,
    modality?: string
  ): Promise<string[]> {
    const { get } = useApiClient();
    const params = new URLSearchParams();
    if (provider) params.append("provider", provider);
    if (modality) params.append("modality", modality);

    const url = params.toString()
      ? `${ApiEndpoints.ADMIN_AGENTS.MODELS}?${params.toString()}`
      : ApiEndpoints.ADMIN_AGENTS.MODELS;

    const response = await get<ApiResponse<string[]>>(url);
    return response.data || [];
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
  static async testConnection(payload: TestConnectionPayload): Promise<any> {
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
  static async testQuickCall(payload: TestQuickCallPayload): Promise<any> {
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
  static async getProfiles(): Promise<{
    env: string;
    profiles: AgentProfile[];
  }> {
    const { get } = useApiClient();
    const response = await get<
      ApiResponse<{ env: string; profiles: AgentProfile[] }>
    >(ApiEndpoints.ADMIN_AGENTS.PROFILES);
    return response.data || { env: "default", profiles: [] };
  }

  /**
   * 获取凭证列表
   */
  static async getCredentials(): Promise<{
    env: string;
    credentials: AgentCredential[];
  }> {
    const { get } = useApiClient();
    const response = await get<
      ApiResponse<{ env: string; credentials: AgentCredential[] }>
    >(ApiEndpoints.ADMIN_AGENTS.CREDENTIALS);
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
}
