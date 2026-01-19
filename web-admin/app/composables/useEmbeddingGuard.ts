import { useAISettingsStore } from "~/stores/aiSettings";
import { useEnvStore } from "~/stores/envStore";
import { useConfirm } from "~/composables/useConfirm";
import { formatEmbeddingGuardMessage, isEmbeddingProfileReady } from "~/utils/knowledge-spaces/embeddingGate";

export type EmbeddingGuardResult = {
  env: string;
  active: any;
  profile: any;
  provider: string;
  model: string;
  embeddingProfileKey: string;
};

const READY_CACHE_TTL_MS = 30000;
const NOT_READY_TTL_MS = 10000;
let cachedReady: EmbeddingGuardResult | null = null;
let cachedReadyAt = 0;
let cachedNotReadyAt = 0;

export const useEmbeddingGuard = () => {
  const aiSettingsStore = useAISettingsStore();
  const envStore = useEnvStore();
  const { confirm } = useConfirm();

  const noKeyProviders = new Set([
    "hash",
    "openai_compatible",
    "openai-compatible",
    "openai_compat",
    "ollama",
    "sentence_transformers",
    "sentence-transformers",
    "sbert",
  ]);

  const ensureEmbeddingReady = async (): Promise<EmbeddingGuardResult | null> => {
    if (!process.client) {
      return {
        env: "dev",
        active: null,
        profile: null,
        provider: "",
        model: "",
        embeddingProfileKey: "",
      };
    }
    const now = Date.now();
    if (cachedReady && now - cachedReadyAt < READY_CACHE_TTL_MS) {
      return cachedReady;
    }
    if (cachedNotReadyAt > 0 && now - cachedNotReadyAt < NOT_READY_TTL_MS) {
      return null;
    }

    const env = envStore.currentEnv || aiSettingsStore.currentEnv || "dev";
    const active = await aiSettingsStore.fetchActiveProfile(env, "embedding");
    const ready = isEmbeddingProfileReady(active);
    let message = formatEmbeddingGuardMessage(active);
    const profile = active?.profile || active || null;
    const provider = String(profile?.provider || "").trim().toLowerCase();
    const model = String(profile?.model || "").trim();
    if (ready) {
      if (!provider || noKeyProviders.has(provider)) {
        cachedReady = {
          env,
          active,
          profile,
          provider,
          model,
          embeddingProfileKey: provider && model ? `${provider}/${model}` : "",
        };
        cachedReadyAt = Date.now();
        return cachedReady;
      }
      try {
        await aiSettingsStore.fetchCredentials(env);
        const credential = aiSettingsStore.getCredentialByProvider(provider);
        if (credential) {
          cachedReady = {
            env,
            active,
            profile,
            provider,
            model,
            embeddingProfileKey: provider && model ? `${provider}/${model}` : "",
          };
          cachedReadyAt = Date.now();
          return cachedReady;
        }
        const label = provider && model ? `${provider}/${model}` : "当前选中的 embedding 模型";
        message = `${label} 当前缺少可用凭证，请先在 AI Settings 保存凭证并测试。`;
      } catch {
        // ignore and fall through
      }
    }
    const ok = await confirm({
      title: "需先配置 embedding",
      message,
      confirmLabel: "去 AI Settings",
      cancelLabel: "暂不",
      tone: "warning",
    });
    if (ok) {
      await navigateTo({ path: "/settings/ai", query: { modality: "embedding" } });
    }
    cachedNotReadyAt = Date.now();
    return null;
  };

  return { ensureEmbeddingReady };
};
