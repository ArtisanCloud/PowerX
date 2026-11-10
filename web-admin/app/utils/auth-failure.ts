/** 只认可当前用户会话的终止码；服务凭据、权限和未分类 401 均不能触发退出。 */
const terminalCodes = new Set(['AUTH_REFRESH_INVALID', 'AUTH_SESSION_INVALID', 'AUTH_SESSION_EXPIRED', 'AUTH_SESSION_REVOKED']);
export function authFailureCode(error: any): string {
  const body = error?.data ?? error?.response?._data;
  const value = body?.error_code ?? body?.reason_code ?? body?.error?.code ?? error?.code;
  return typeof value === 'string' ? value : '';
}
export function authFailureStatus(error: any): number {
  return Number(error?.status ?? error?.statusCode ?? error?.response?.status ?? 0);
}
export function isTerminalAuthFailure(error: unknown): boolean {
  return authFailureStatus(error) === 401 && terminalCodes.has(authFailureCode(error));
}
export function isUserSessionReport(code: unknown): code is 'IAM_CONTEXT_USER_SESSION_INVALID' {
  return code === 'IAM_CONTEXT_USER_SESSION_INVALID';
}

/** 登录返回地址取宿主路由，拒绝外部地址和协议相对地址。 */
export function loginRecoveryLocation(path: string): string {
  const safe = typeof path === 'string' && path.startsWith('/') && !path.startsWith('//') && !/[\\\u0000-\u001f]/.test(path) ? path : '/home';
  if (/^\/(?:[a-z]{2}(?:-[A-Z]{2})?\/)?users\/login(?:[/?#]|$)/.test(safe)) return safe;
  return `/users/login?redirect=${encodeURIComponent(safe)}`;
}
