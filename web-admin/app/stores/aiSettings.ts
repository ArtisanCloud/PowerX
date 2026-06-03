import { defineStore } from "pinia";
import {
  AISettingService,
  type AgentProfile,
  type AgentCredential,
  type SaveSettingsPayload,
  type Provider,
} from "~/composables/api/services/aiSettingService";

const compactError = (raw: unknown): string => {
  const message = String(raw ?? "").trim();
  if (!message) return "未知错误";

  const lower = message.toLowerCase();
  if (lower.includes("ollama_model_not_found")) {
    return "Ollama model not found";
  }
  if (
    lower.includes("accessdenied.unpurchased") ||
    lower.includes("access to model denied")
  ) {
    return "模型未开通或无权限（AccessDenied.Unpurchased），请在官方控制台开通该模型后重试。";
  }
  if (
    lower.includes("invalid_api_key") ||
    lower.includes("incorrect api key provided")
  ) {
    return "API Key 无效，请确认使用的是对应站点/地域的官方 Key。";
  }
  if (lower.includes("unknown image provider")) {
    return "当前 Provider 的图像驱动未启用，请联系管理员检查模型驱动配置。";
  }

  const bodyPos = message.indexOf("body=");
  let brief = bodyPos >= 0 ? message.slice(0, bodyPos).trim() : message;
  brief = brief.replace(/\s+/g, " ");
  if (brief.length > 180) {
    brief = `${brief.slice(0, 180)}...`;
  }
  return brief || "请求失败，请稍后重试";
};

const extractErrorDetail = (raw: unknown): string => {
  const message = String(raw ?? "").trim();
  if (!message) return "";
  return message.length > 4000 ? `${message.slice(0, 4000)}...` : message;
};

export interface AISettingsState {
  providers: Provider[];
  models: string[];
  profiles: AgentProfile[];
  activeProfile: AgentProfile | null;
  credentials: AgentCredential[];
  currentEnv: string;
  loading: boolean;
  saving: boolean;
  testing: boolean;
  lastTestMessage: string;
  lastTestDetail: string;
  initialized: boolean;
}

export const useAISettingsStore = defineStore("aiSettings", {
  state: (): AISettingsState => ({
    providers: [],
    models: [],
    profiles: [],
    activeProfile: null,
    credentials: [],
    currentEnv: "default",
    loading: false,
    saving: false,
    testing: false,
    lastTestMessage: "",
    lastTestDetail: "",
    initialized: false,
  }),

  getters: {
    /**
     * 根据模态获取配置文件
     */
    getProfileByModality: (state) => (modality: string) => {
      return (
        (state.profiles ?? []).find?.(
          (profile) => profile.modality === modality
        ) ?? null
      );
    },

    /**
     * 根据供应商获取凭证
     */
    getCredentialByProvider: (state) => (provider: string) => {
      return (
        (state.credentials ?? []).find?.(
          (credential) =>
            credential.provider.toLowerCase() === provider.toLowerCase()
        ) ?? null
      );
    },

    /**
     * 检查是否有配置
     */
    hasConfiguration: (state) => (modality: string) => {
      const profile = (state.profiles ?? []).find?.(
        (p) => p.modality === modality
      );
      const credential = profile
        ? (state.credentials ?? []).find?.(
            (c) => c.provider.toLowerCase() === profile.provider.toLowerCase()
          )
        : null;
      return !!(profile && credential);
    },
  },

  actions: {
    /**
     * 初始化数据
     */
    async initialize(env: string = "default") {
      // 防止重复初始化
      if (this.initialized) {
        this.setCurrentEnv(env);
        return;
      }

      this.loading = true;
      try {
        this.currentEnv = env || this.currentEnv || "default";
        const [providers, profiles, credentials] = await Promise.all([
          AISettingService.getProviders(),
          AISettingService.getProfiles(this.currentEnv),
          AISettingService.getCredentials(this.currentEnv),
        ]);

        // 确保数据结构正确，添加兜底
        this.providers = providers ?? <Provider[]>[];
        this.profiles = profiles?.profiles ?? [];
        this.credentials = credentials?.credentials ?? [];
        this.currentEnv =
          profiles?.env || credentials?.env || this.currentEnv || "default";

        // console.info("AI store设置初始化成功", {
        //   providers: this.providers.length,
        //   profiles: this.profiles.length,
        //   credentials: this.credentials.length,
        // });

        // 添加调试日志
        // console.info(
        //   "providers after init in store",
        //   JSON.stringify(this.providers)
        // );

        // 可选：获取默认的激活配置（LLM 模态）
        try {
          const resActiveProfile = await this.fetchActiveProfile(
            "default",
            "llm"
          );
          if (resActiveProfile) {
            this.activeProfile = resActiveProfile.profile;
            // console.info("默认激活配置加载成功", resActiveProfile);
          }
        } catch (error) {
          console.warn("获取默认激活配置失败，将使用现有配置", error);
        }

        // 标记为已初始化
        this.initialized = true;
      } catch (error) {
        console.error("初始化AI设置失败:", error);
        // 保底：保证是数组，避免后续 .find/.length 崩掉
        this.providers = this.providers ?? [];
        this.profiles = this.profiles ?? [];
        this.credentials = this.credentials ?? [];
        throw error;
      } finally {
        this.loading = false;
      }
    },

    setCurrentEnv(env: string) {
      if (env && env !== this.currentEnv) {
        this.currentEnv = env;
      }
    },

    async refreshEnvData(env: string, modalities?: string[]) {
      const targetEnv = env || this.currentEnv || "default";
      const [profiles, credentials] = await Promise.all([
        AISettingService.getProfiles(targetEnv, modalities),
        AISettingService.getCredentials(targetEnv),
      ]);
      this.profiles = profiles?.profiles ?? [];
      this.credentials = credentials?.credentials ?? [];
      this.currentEnv = profiles?.env || credentials?.env || targetEnv;
      return { profiles, credentials };
    },

    /**
     * 获取供应商列表
     */
    async fetchProviders(modality?: string, env?: string) {
      try {
        const targetEnv = env || this.currentEnv || "default";
        const normModality = modality ? this.mapModality(modality) : undefined;
        const providers = await AISettingService.getProviders(normModality, targetEnv);
        this.providers = providers;
      } catch (error) {
        console.error("获取供应商列表失败:", error);
        throw error;
      }
    },

    /**
     * 获取模型列表
     */
    async fetchModels(provider?: string, modality?: string, env?: string, app?: string) {
      // 关键：参数不全就短路，但不清空 models
      if (!provider || !modality) {
        console.info("fetchModels -> 参数不全，跳过:", { provider, modality });
        return;
      }

      try {
        // ✅ 参数规范化
        const normProvider = provider.trim().toLowerCase(); // "OpenAI" -> "openai"
        const normModality = this.mapModality(modality);
        const normEnv = env || this.currentEnv || "default";

        const res = await AISettingService.getModels(
          normProvider,
          normModality,
          normEnv,
          app
        );

        // ✅ 打印原始响应
        // console.info("raw models response", JSON.stringify(res));

        // ✅ 容错取值
        if (Array.isArray(res)) {
          this.models = res;
        } else {
          const data = res as any;
          this.models = data?.models ?? data?.items ?? [];
        }
        // console.info("store.models set to", this.models);
      } catch (error) {
        console.error("获取模型列表失败:", error);
        // 发生错误时才清空 models
        this.models = [];
        throw error;
      }
    },

    /**
     * 模态映射
     */
    mapModality(modality: string): string {
      const v = String(modality || "").trim().toLowerCase();
      switch (v) {
        // 后端 contract.Modality
        case "llm":
          return "llm";
        case "image":
          return "image";
        case "embedding":
          return "embedding";
        case "audio_tts":
          return "audio_tts";
        case "audio_asr":
          return "audio_asr";
        case "video":
          return "video";
        case "model3d":
        case "model_3d":
        case "3d":
          return "model3d";
        case "rerank":
          return "rerank";

        // 兼容旧/别名输入
        case "chat":
        case "text":
        case "completion":
          return "llm";
        case "tts":
          return "audio_tts";
        case "asr":
          return "audio_asr";
        default:
          return v || modality;
      }
    },

    /**
     * 获取配置文件
     */
    async fetchProfiles(env?: string, modalities?: string[]) {
      try {
        const response = await AISettingService.getProfiles(
          env || this.currentEnv,
          modalities
        );
        this.profiles = response.profiles;
        this.currentEnv = response.env || this.currentEnv;
      } catch (error) {
        console.error("获取配置文件失败:", error);
        throw error;
      }
    },

    /**
     * 获取凭证
     */
    async fetchCredentials(env?: string) {
      try {
        const response = await AISettingService.getCredentials(
          env || this.currentEnv
        );
        this.credentials = response.credentials;
        if (response.env) {
          this.currentEnv = response.env;
        }
      } catch (error) {
        console.error("获取凭证失败:", error);
        throw error;
      }
    },

    /**
     * 保存设置
     */
    async saveSettings(payload: SaveSettingsPayload) {
      this.saving = true;
      try {
        const targetEnv = payload.env || this.currentEnv || "default";
        const nextPayload = {
          ...payload,
          env: targetEnv,
        };
        // 转换字段名以匹配后端期望的格式
        const result = await AISettingService.saveSettings(nextPayload);
        if (result.ok) {
          const modality = String(payload.modality || "").trim();
          const modalityPayload = (payload as any)[modality] || null;
          const provider = String(modalityPayload?.provider || "").trim();
          const model = String(modalityPayload?.model || "").trim();
          const app = String(modalityPayload?.app || "").trim();
          const routedModel = app ? `${app}:${model}` : model;
          if (modality && provider && model) {
            try {
              await AISettingService.setActiveProfile({
                env: targetEnv,
                modality,
                provider,
                model: routedModel,
              });
            } catch (error) {
              console.warn("设置激活配置失败，将保持当前路由默认值", error);
            }
          }
          // 重新获取配置文件和凭证
          await Promise.all([
            this.fetchProfiles(targetEnv),
            this.fetchCredentials(targetEnv),
          ]);
          this.lastTestMessage = "设置保存成功";
        }
        return result;
      } catch (error) {
        console.error("保存设置失败:", error);
        this.lastTestMessage = `保存失败: ${error instanceof Error ? error.message : "未知错误"}`;
        throw error;
      } finally {
        this.saving = false;
      }
    },

    /**
     * 测试连接
     */
    async testConnection(provider: string, payload: any) {
      this.testing = true;
      try {
        const nextPayload = {
          ...payload,
          env: payload.env || this.currentEnv || "default",
        };
        const result = await AISettingService.testConnection(nextPayload);
        this.lastTestMessage = `连接测试成功 - Provider: ${provider}`;
        this.lastTestDetail = "";
        return result;
      } catch (error) {
        console.error("连接测试失败:", error);
        const raw = error instanceof Error ? error.message : error;
        this.lastTestMessage = `连接测试失败: ${compactError(raw)}`;
        this.lastTestDetail = extractErrorDetail(raw);
        throw error;
      } finally {
        this.testing = false;
      }
    },

    /**
     * 测试快速调用
     */
    async testQuickCall(
      provider: string,
      model: string,
      payload: any,
      message = "Hello, this is a test message."
    ) {
      this.testing = true;
      try {
        const result = await AISettingService.testQuickCall({
          ...payload,
          env: payload.env || this.currentEnv || "default",
          prompt: message,
        });
        this.lastTestMessage = `快速调用测试成功 - Model: ${model}`;
        this.lastTestDetail = "";
        return result;
      } catch (error) {
        console.error("快速调用测试失败:", error);
        const raw = error instanceof Error ? error.message : error;
        this.lastTestMessage = `快速调用测试失败: ${compactError(raw)}`;
        this.lastTestDetail = extractErrorDetail(raw);
        throw error;
      } finally {
        this.testing = false;
      }
    },

    /**
     * 清除测试消息
     */
    clearTestMessage() {
      this.lastTestMessage = "";
      this.lastTestDetail = "";
    },

    /**
     * 获取当前激活的配置
     */
    async fetchActiveProfile(
      env: string = "default",
      modality: string = "llm"
    ) {
      // console.info("AI Settings Store: 获取激活配置", { env, modality });
      try {
        const response = await AISettingService.getActiveProfile(env, modality);
        // console.info("AI Settings Store: 激活配置响应", response);

        if (response.code === 200 && response.data) {
          return response.data;
        } else {
          console.error(
            "AI Settings Store: 获取激活配置失败",
            response.message
          );
          return null;
        }
      } catch (error) {
        console.error("AI Settings Store: 获取激活配置异常", error);
        return null;
      }
    },
  },
});
