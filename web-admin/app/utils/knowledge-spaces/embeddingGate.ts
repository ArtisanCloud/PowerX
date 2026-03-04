type EmbeddingProfileLike = {
  defaults?: Record<string, unknown> | null;
  Defaults?: Record<string, unknown> | null;
  capCache?: Record<string, unknown> | null;
  cap_cache?: Record<string, unknown> | null;
};

const parseDimension = (value: unknown): number => {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string") {
    const parsed = Number.parseInt(value.trim(), 10);
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
};

const extractDimension = (payload: Record<string, unknown> | null | undefined): number => {
  if (!payload) return 0;
  if ("dimensions" in payload) {
    const dim = parseDimension(payload.dimensions);
    if (dim > 0) return dim;
  }
  if ("dim" in payload) {
    const dim = parseDimension(payload.dim);
    if (dim > 0) return dim;
  }
  return 0;
};

export const resolveEmbeddingDimensions = (profile: EmbeddingProfileLike | null | undefined): number => {
  if (!profile) return 0;
  const defaults = profile.defaults || profile.Defaults || null;
  const capCache = profile.capCache || profile.cap_cache || null;
  return extractDimension(defaults) || extractDimension(capCache);
};

const hasProbeStamp = (profile: EmbeddingProfileLike | null | undefined): boolean => {
  if (!profile) return false;
  const capCache = profile.capCache || profile.cap_cache;
  if (!capCache) return false;
  const stamp = (capCache as Record<string, unknown>)["probed_at"];
  if (typeof stamp === "string") return stamp.trim().length > 0;
  return Boolean(stamp);
};

export const isEmbeddingProfileReady = (active: any): boolean => {
  if (!active) return false;
  if (active.configured === false) return false;
  const profile = (active.profile || active) as EmbeddingProfileLike | null;
  if (!profile) return false;
  if (resolveEmbeddingDimensions(profile) <= 0) return false;
  return hasProbeStamp(profile);
};

export const formatEmbeddingGuardMessage = (active: any): string => {
  if (!active || active.configured === false || !active.profile) {
    return "当前租户尚未配置可用的 embedding 模型，请先在 AI Settings 里完成配置并测试。";
  }
  const profile = active.profile as { provider?: string; model?: string };
  const provider = String(profile?.provider || "").trim();
  const model = String(profile?.model || "").trim();
  const label = provider && model ? `${provider}/${model}` : "当前选中的 embedding 模型";
  return `${label} 当前选中的 embedding 尚未测试通过，请先在 AI Settings 里执行测试。`;
};
