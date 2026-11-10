import { describe, expect, it } from 'vitest';
import { visionDocumentationURL, visionModelOptions, visionUsage } from '~/utils/ai-vision-model-choice';
import type { OnlineCatalogModel } from '~/composables/api/services/aiSettingService';

const entry = (data: Partial<OnlineCatalogModel>) => ({ id: 'native', label: 'Native', modalities: [], reported_modalities: [], manual_modalities: [], match: 'unmatched', ...data }) as OnlineCatalogModel;
describe('视觉型号用途与在线 ID 分离', () => {
  it('只展示在线返回的 ID，不通过官方用途资料补入本地型号', () => {
    const items = [entry({ id: 'native', vision: { status: 'supported', source: 'official_docs' } }), entry({ id: 'tts', vision: { status: 'unsupported', source: 'official_docs' } })];
    const options = visionModelOptions(['tts', 'native', 'fresh', 'native'], items);
    expect(options.map(item => item.value)).toEqual(['tts', 'native', 'fresh']);
    expect(options.map(item => item.disabled)).toEqual([true, false, false]);
    expect(options[2].vision?.status).toBe('unknown');
  });
  it('旧 Core 响应只根据明确 I/O 判断；仅有型号名称或文字输出保持未知', () => {
    expect(visionUsage(entry({ id: 'gpt-vision-guessed' })).status).toBe('unknown');
    expect(visionUsage(entry({ input_modalities: ['image'], output_modalities: ['text'] }))).toMatchObject({ status: 'supported', source: 'provider_api' });
    expect(visionUsage(entry({ input_modalities: ['text'], output_modalities: ['audio'] })).reason_code).toBe('AI_VISION_IMAGE_INPUT_UNSUPPORTED');
    expect(visionUsage(entry({ input_modalities: ['image'], output_modalities: ['image'] })).reason_code).toBe('AI_VISION_TEXT_OUTPUT_UNSUPPORTED');
    expect(visionUsage(entry({ output_modalities: ['text'] })).status).toBe('unknown');
  });
  it('文档链接仅允许 OpenAI 官方型号页面', () => {
    expect(visionDocumentationURL('https://developers.openai.com/api/docs/models/gpt-5.4-mini')).toContain('/models/gpt-5.4-mini');
    for (const url of ['javascript:alert(1)', 'https://evil.invalid', 'https://developers.openai.com.evil.invalid/api/docs/models/x']) expect(visionDocumentationURL(url)).toBeUndefined();
  });
});
