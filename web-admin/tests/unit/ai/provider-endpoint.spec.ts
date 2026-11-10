import { describe, expect, it } from 'vitest';
import { defaultAIProviderBaseURL, resolveAIFormBaseURL } from '~/utils/ai-provider-endpoint';

describe('表单清空地址恢复当前接入方式默认值', () => {
  const provider = { id: 'example', auth: { defaults: { base_url: 'https://default.invalid/v1' }, modes: [
    { id: 'openai', defaults: { base_url: 'https://openai-mode.invalid/v1' } },
    { id: 'tc3', defaults: { base_url: 'https://tc3-mode.invalid' } },
  ] } };
  it('清空与空白使用配置中的默认地址，指定地址保持原值', () => {
    expect(resolveAIFormBaseURL('', provider, 'openai')).toBe('https://openai-mode.invalid/v1');
    expect(resolveAIFormBaseURL('  ', provider, 'tc3')).toBe('https://tc3-mode.invalid');
    expect(resolveAIFormBaseURL(' https://custom.invalid/v1 ', provider, 'tc3')).toBe('https://custom.invalid/v1');
    expect(defaultAIProviderBaseURL({ id: 'openai', auth: { defaults: { base_url: 'https://api.openai.com/v1' } } })).toBe('https://api.openai.com/v1');
  });
  it('没有默认地址时不虚构入口，并保留本地协议默认值', () => {
    expect(defaultAIProviderBaseURL({ id: 'openai_compatible' })).toBe('');
    expect(defaultAIProviderBaseURL({ id: 'ollama' })).toBe('http://127.0.0.1:11434');
    expect(defaultAIProviderBaseURL({ id: 'ollama', auth: { defaults: { base_url: 'http://configured.invalid:11434' } } })).toBe('http://configured.invalid:11434');
  });
});
