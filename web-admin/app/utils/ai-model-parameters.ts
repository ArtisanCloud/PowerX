import type { ImageOperation, ModelParameterProfile } from '~/composables/api/services/aiSettingService';

export const modelParameterPresets: Record<string, Record<string, unknown>> = {
  llm: { temperature: .7, topP: 1, maxTokens: 4096, stream: true, maxConcurrentRequests: 2, reasoningEffort: '' },
  vlm: { maxTokens: 2048, reasoningEffort: 'none', temperature: null, topP: null, jsonMode: false },
  image: { size: '1024x1024', quality: '', format: 'png', promptHint: '' },
};
const aliases: Record<string, string> = { max_tokens: 'maxTokens', top_p: 'topP', reasoning_effort: 'reasoningEffort', max_concurrent_requests: 'maxConcurrentRequests', json_mode: 'jsonMode' };

export function modelParameterDefaults(modality: string, metadata?: ModelParameterProfile, saved?: Record<string, unknown>, operation?: ImageOperation) {
  const result = { ...modelParameterPresets[modality] };
  if (modality === 'image') {
    for (const [key, spec] of Object.entries(operation?.parameters || {})) {
      if (key in result && spec.default !== undefined) result[key] = spec.default;
    }
  }
  for (const values of [metadata?.defaults, saved]) {
    for (const [key, value] of Object.entries(values || {})) {
      const field = aliases[key] || key;
      if (field in result && value !== undefined && value !== null) result[field] = value;
    }
  }
  return result;
}

export function imageParameterOptions(operation: ImageOperation | undefined, parameter: string): string[] {
  return (operation?.parameters?.[parameter]?.enum || []).map(String);
}

export function invalidImageParameters(operation: ImageOperation | undefined, values: Record<string, unknown>) {
  return Object.entries(operation?.parameters || {}).flatMap(([parameter, spec]) => {
    const value = values[parameter];
    if (spec.enum?.length && value !== undefined && value !== '' && !spec.enum.map(String).includes(String(value))) {
      return [{ parameter, value: String(value), allowed: spec.enum.map(String) }];
    }
    return [];
  });
}

export function modelSupportsParameter(metadata: ModelParameterProfile | undefined, field: string, effort?: string) {
  if (!metadata) return true;
  if (metadata.supported_parameters_known && !metadata.supported_parameters?.includes(field)) return false;
  if (field !== 'temperature' && field !== 'top_p') return true;
  if (metadata.sampling === 'unsupported') return false;
  return metadata.sampling !== 'reasoning_none' || (effort || metadata.defaults.reasoning_effort) === 'none';
}

export function modelParameterKey(env: string, modality: string, provider: string, app: string, model: string) {
  return JSON.stringify([env, modality, provider, app, model]);
}
