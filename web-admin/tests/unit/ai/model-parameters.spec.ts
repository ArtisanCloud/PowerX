import { describe, expect, it } from 'vitest';
import { imageParameterOptions, invalidImageParameters, modelParameterDefaults, modelParameterKey, modelSupportsParameter } from '~/utils/ai-model-parameters';
import type { ModelParameterProfile } from '~/composables/api/services/aiSettingService';
const luna: ModelParameterProfile = { version: 1, provider: 'openai', model: 'gpt-6-luna', source: 'official_docs', defaults: { reasoning_effort: 'medium' }, sampling: 'reasoning_none', reasoning_efforts: ['none','low','medium','high'] };

describe('逐型号参数与来源', () => {
 it('生图合同决定默认值和选项，保留合法覆盖与可见的非法旧值', () => {
  const operation = { driver: 'openai', parameters: { quality: { type: 'string', enum: ['auto','low','medium','high'], default: 'auto' }, size: { type: 'string', default: '1024x1024' } } };
  expect(modelParameterDefaults('image', undefined, undefined, operation).quality).toBe('auto');
  expect(modelParameterDefaults('image', undefined, { quality: 'high' }, operation).quality).toBe('high');
  const old = modelParameterDefaults('image', undefined, { quality: 'standard' }, operation);
  expect(old.quality).toBe('standard');
  expect(invalidImageParameters(operation, old)).toEqual([{parameter:'quality',value:'standard',allowed:['auto','low','medium','high']}]);
  expect(imageParameterOptions(operation,'quality')).not.toContain('standard');
  const other = { driver:'other', parameters:{ quality:{type:'string',enum:['standard','hd'],default:'standard'} } };
  expect(modelParameterDefaults('image', undefined, undefined, other).quality).toBe('standard');
  expect(imageParameterOptions(other,'quality')).toEqual(['standard','hd']);
  expect(modelParameterDefaults('image').quality).toBe('');
 });
 it('官方默认、平台预算与用户覆盖分开，显式零值保留', () => {
  const values = modelParameterDefaults('llm', luna, { temperature: 0, maxTokens: 777, topP: .8, reasoning_effort: 'none', api_key: 'must-not-copy' });
  expect(values).toMatchObject({ temperature: 0, maxTokens:777, topP:.8, reasoningEffort:'none' });
  expect(values).not.toHaveProperty('api_key'); expect(luna.defaults).not.toHaveProperty('max_tokens');
  expect(modelParameterDefaults('llm',luna)).toMatchObject({maxTokens:4096,reasoningEffort:'medium'});
 });
 it('按推理模式禁用采样，并尊重供应商明确的支持清单', () => {
  expect(modelSupportsParameter(luna,'temperature','medium')).toBe(false);
  expect(modelSupportsParameter(luna,'temperature','none')).toBe(true);
  expect(modelSupportsParameter({...luna,sampling:'unsupported'},'temperature','none')).toBe(false);
  expect(modelSupportsParameter({...luna,supported_parameters_known:true,supported_parameters:['temperature']},'top_p','none')).toBe(false);
  expect(modelSupportsParameter(undefined,'temperature')).toBe(true);
 });
 it('缓存身份包含环境、模态、供应商、应用和型号', () => {
  const key = modelParameterKey('dev','llm','openai','','model-a');
  for(const args of [['prod','llm','openai','','model-a'],['dev','vlm','openai','','model-a'],['dev','llm','qwen','','model-a'],['dev','llm','openai','app','model-a'],['dev','llm','openai','','model-b']]) expect(modelParameterKey(...args as [string,string,string,string,string])).not.toBe(key);
 });
});
