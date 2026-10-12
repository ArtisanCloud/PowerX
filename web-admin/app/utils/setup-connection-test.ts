export type SetupConnectionKind = 'database' | 'cache';

const object = (value: unknown): Record<string, any> =>
  value !== null && typeof value === 'object' ? value as Record<string, any> : {};

/** Redis RESP 错误码是协议字段；不从翻译后的提示猜测认证状态。 */
export function describeSetupConnectionError(error: unknown, kind: SetupConnectionKind) {
  const failure = object(error);
  const response = object(failure.response);
  const data = object(failure.data ?? response._data);
  const nested = object(data.error);
  const detail = [data.error, nested.message, data.message, failure.message]
    .find(value => typeof value === 'string' && value.trim()) as string | undefined;
  const redisCode = kind === 'cache' ? detail?.trim().match(/^(NOAUTH|WRONGPASS|NOPERM)(?:\s|$)/)?.[1] : undefined;
  const messages: Record<string, string> = {
    NOAUTH: 'setup.connection.redisAuthRequired',
    WRONGPASS: 'setup.connection.redisAuthInvalid',
    NOPERM: 'setup.connection.redisPermissionDenied',
  };
  const headers = response.headers;
  const traceId = typeof data.trace_id === 'string' ? data.trace_id
    : typeof headers?.get === 'function' ? headers.get('x-trace-id') : null;
  return {
    messageKey: redisCode ? messages[redisCode] : undefined,
    detail,
    diagnosticId: traceId || (typeof data.request_id === 'string' ? data.request_id : ''),
  };
}
