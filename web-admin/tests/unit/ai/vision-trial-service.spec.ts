import { beforeEach, describe, it, expect, vi } from 'vitest';
import { AISettingService } from '~/composables/api/services/aiSettingService';
const { post } = vi.hoisted(() => ({ post: vi.fn() }));
vi.mock('~/composables/api/index', () => ({ useApiClient: () => ({ post }) }));
beforeEach(() => post.mockReset());
it('图片试跑使用正式管理测试入口和显式执行字段，禁用短超时和自动重试', async () => {
 const result = { type: 'analysis', provider: 'openai', model: 'gpt-6-luna', text: 'A picture.', trace_id: 'trace', elapsed_ms: 4, env: 'dev' };
 post.mockResolvedValueOnce({ data: { ok: true, generation_submitted: true, result } });
 const signal = new AbortController().signal;
 const value = await AISettingService.trialVision({ env: 'dev', provider: 'openai', app: '', model: 'gpt-6-luna', params: { maxTokens: 2048, jsonMode: false } }, { type: 'image_url', url: 'https://example.invalid/image.png' }, 'Describe', signal);
 expect(post).toHaveBeenCalledExactlyOnceWith('/admin/agents/test/call', { env: 'dev', modality: 'vlm', execute: true, vlm: { provider: 'openai', app: '', model: 'gpt-6-luna', reuseCredential: true, maxTokens: 2048, jsonMode: false }, inputs: [{ type: 'text', content: 'Describe' }, { type: 'image_url', url: 'https://example.invalid/image.png' }] }, { signal, timeout: 0, retry: 0, useGlobalLoading: false, useGlobalError: false });
 expect(value).toEqual(result);
});
it('连接成功或仅通过参数检查不能冒充真实视觉试跑成功', async () => {
 post.mockResolvedValueOnce({ data: { ok: true, generation_submitted: false } });
 await expect(AISettingService.trialVision({ env: 'dev', provider: 'openai', app: '', model: 'native', params: { maxTokens: 2048, jsonMode: false } }, { type: 'image_url', url: 'https://example.invalid/i.png' }, 'Describe', new AbortController().signal)).rejects.toMatchObject({ errorCode: 'AI_VISION_TRIAL_NOT_SUPPORTED' });
});

it('当前表单 Key、地址和型号优先，不要求先保存配置', async () => {
 const result = { type: 'analysis', provider: 'openai', model: 'unsaved', text: 'A picture.', trace_id: 'trace', elapsed_ms: 4, env: 'dev' };
 post.mockResolvedValue({ data: { ok: true, generation_submitted: true, result } });
 const connection = { apiKey: 'draft-private-fixture', baseURL: 'https://draft.example.invalid/v1', organization: 'draft-org', authMode: '', region: '', secretId: '', azureDeployment: '' };
 await AISettingService.trialVision({ env: 'dev', provider: 'openai', app: '', model: 'unsaved', connection, params: { maxTokens: 3072, jsonMode: false } }, { type: 'image_url', url: 'https://example.invalid/i.png' }, 'Describe', new AbortController().signal);
 expect(post.mock.calls[0][1]).toMatchObject({ useFormConnection: true, vlm: { ...connection, model: 'unsaved', maxTokens: 3072 } });
 expect(post).toHaveBeenCalledTimes(1);
 expect(post.mock.calls[0][1].vlm).not.toHaveProperty('reuseCredential');
 post.mockClear();
 const { apiKey, ...blank } = connection;
 await AISettingService.trialVision({ env: 'dev', provider: 'openai', app: '', model: 'unsaved', connection: { ...blank, baseURL: '' }, params: { maxTokens: 3072, jsonMode: false } }, { type: 'image_url', url: 'https://example.invalid/i.png' }, 'Describe', new AbortController().signal);
 expect(post.mock.calls[0][1]).toMatchObject({ useFormConnection: true, vlm: { baseURL: '' } });
 expect(post.mock.calls[0][1].vlm).not.toHaveProperty('apiKey');
});
