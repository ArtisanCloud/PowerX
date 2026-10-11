export function formatBackupTime(value?: string, locale = 'zh-CN'): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(locale, {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  }).format(date);
}

export function backupFileName(uri?: string): string {
  const name = String(uri || '').split(/[\\/]/).pop() || '';
  try { return decodeURIComponent(name); } catch { return name; }
}

export function backupStatusKey(status: string): string | null {
  return ['pending', 'running', 'success', 'failed', 'cancelled'].includes(status)
    ? `backupCenter.workspace.statuses.${status}` : null;
}
