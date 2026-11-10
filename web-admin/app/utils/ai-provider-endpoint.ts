type ProviderEndpoint = {
  id?: string;
  auth?: {
    defaults?: Record<string, string>;
    modes?: Array<{ id: string; defaults?: Record<string, string> }>;
  };
};

// 表单留空代表当前 Provider／接入方式的默认地址，不使用已保存的旧地址。
export function defaultAIProviderBaseURL(provider: ProviderEndpoint | null | undefined, authMode = ""): string {
  const modes = provider?.auth?.modes || [];
  const mode = modes.find(item => item.id.toLowerCase() === authMode.trim().toLowerCase()) || modes[0];
  const configured = String(mode?.defaults?.base_url || provider?.auth?.defaults?.base_url || "").trim();
  if (configured) return configured;
  const id = String(provider?.id || "").toLowerCase();
  if (id === "ollama") return "http://127.0.0.1:11434";
  if (id === "huggingface" || id === "hf") return "https://router.huggingface.co/v1";
  return "";
}

export function resolveAIFormBaseURL(value: unknown, provider: ProviderEndpoint | null | undefined, authMode = ""): string {
  return String(value ?? "").trim() || defaultAIProviderBaseURL(provider, authMode);
}
