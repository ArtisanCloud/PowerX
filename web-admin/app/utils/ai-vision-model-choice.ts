import type { OnlineCatalogModel, VisionModelUsage } from '~/composables/api/services/aiSettingService';

export type VisionModelOption = { label: string; value: string | number | null; disabled?: boolean; vision?: VisionModelUsage };

// 兼容尚未重启的 Core：旧响应只按接口明确的 I/O 分类，绝不凭名称猜测用途。
export function visionUsage(item?: OnlineCatalogModel): VisionModelUsage {
  if (item?.vision) return item.vision;
  if (item?.input_modalities?.length && item?.output_modalities?.length) {
    if (!item.input_modalities.includes('image')) return { status: 'unsupported', source: 'provider_api', reason_code: 'AI_VISION_IMAGE_INPUT_UNSUPPORTED' };
    if (!item.output_modalities.includes('text')) return { status: 'unsupported', source: 'provider_api', reason_code: 'AI_VISION_TEXT_OUTPUT_UNSUPPORTED' };
    return { status: 'supported', source: 'provider_api' };
  }
  return item?.reported_modalities?.includes('vlm') ? { status: 'supported', source: 'provider_api' } : { status: 'unknown', source: 'unknown' };
}

export function visionModelOptions(models: string[], items: OnlineCatalogModel[] = []): VisionModelOption[] {
  const byID = new Map(items.map(item => [item.id, item]));
  return [...new Set(models)].filter(Boolean).map(id => {
    const usage = visionUsage(byID.get(id));
    return { label: id, value: id, vision: usage, disabled: usage.status === 'unsupported' };
  });
}

export function visionUsageKey(usage?: VisionModelUsage): string {
  return `settings.ai.visionModels.${usage?.status === 'unsupported' ? (usage.reason_code || 'unsupported') : (usage?.status || 'unknown')}`;
}

export function visionDocumentationURL(value?: string): string | undefined {
  if (!value) return;
  try {
    const url = new URL(value);
    if (url.protocol === 'https:' && url.hostname === 'developers.openai.com' && url.pathname.startsWith('/api/docs/models/')) return url.href;
  } catch { /* 非官方链接不开放跳转。 */ }
}
