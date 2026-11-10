export interface ApiKeyOwnerPermissionConflict {
  key_id: string;
  key_name: string;
  binding_mode: string;
  owner_plugin_ids: string[];
  permission_owner_plugin_ids: string[];
  permission_id: number;
  permission_name: string;
  capability_id: string;
  required_plugin_id: string;
  scope: string;
  action: string;
  resource_type: string;
  resource_pattern: string;
}

export interface ApiKeyOwnerConflictReport {
  profile_id: number;
  profile_name: string;
  conflict_count: number;
  conflicting_key_count: number;
  conflicts: ApiKeyOwnerPermissionConflict[];
  truncated: boolean;
  trace_id: string;
}

const strings = (value: unknown): string[] =>
  Array.isArray(value)
    ? value.filter((item): item is string => typeof item === 'string')
    : [];
const text = (value: unknown): string =>
  typeof value === 'string' ? value : '';

// 读取统一 API Client 保留的结构化错误；缺少明细时继续使用原有通用失败提示。
export function parseApiKeyOwnerConflicts(
  error: any,
): ApiKeyOwnerConflictReport | null {
  const data = error?.data ?? error?.response?._data ?? error;
  const details = data?.details ?? error?.details;
  const code =
    data?.error_code ??
    data?.reason_code ??
    error?.errorCode ??
    details?.reason_code;
  if (
    code !== 'API_KEY_PLUGIN_OWNER_CONFLICT' ||
    !Array.isArray(details?.conflicts)
  )
    return null;
  if (
    details.conflicts.some(
      (item: any) =>
        !item ||
        typeof item.key_id !== 'string' ||
        !item.key_id ||
        typeof item.required_plugin_id !== 'string' ||
        !item.required_plugin_id ||
        !Array.isArray(item.owner_plugin_ids),
    )
  )
    return null;
  const conflicts: ApiKeyOwnerPermissionConflict[] = details.conflicts.map(
    (item: any) => ({
      key_id: item.key_id,
      key_name: text(item.key_name),
      binding_mode: text(item.binding_mode),
      owner_plugin_ids: strings(item.owner_plugin_ids),
      permission_owner_plugin_ids: strings(item.permission_owner_plugin_ids),
      permission_id: Number(item.permission_id) || 0,
      permission_name: text(item.permission_name),
      capability_id: text(item.capability_id),
      required_plugin_id: item.required_plugin_id,
      scope: text(item.scope),
      action: text(item.action),
      resource_type: text(item.resource_type),
      resource_pattern: text(item.resource_pattern),
    }),
  );
  if (!conflicts.length) return null;
  return {
    profile_id: Number(details.profile_id) || 0,
    profile_name: text(details.profile_name),
    conflict_count: Number(details.conflict_count) || conflicts.length,
    conflicting_key_count:
      Number(details.conflicting_key_count) ||
      new Set(conflicts.map((item) => item.key_id)).size,
    conflicts,
    truncated: details.truncated === true,
    trace_id: text(details.trace_id) || text(data?.request_id),
  };
}
