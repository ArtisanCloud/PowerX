export interface InstallPackageIdentity { id: string; version: string }
export interface InstallInspection {
  plugin_id: string;
  installation: { version?: string; revision?: string; state?: string; ready: boolean; tenant_ready?: boolean; runtime_ready?: boolean; reason?: string };
}
export interface InstallRecovery {
  target?: InstallPackageIdentity;
  force: boolean;
  enable: boolean;
  baselineRevision?: string;
  receiptRevision?: string;
  inspection?: InstallInspection;
  outcome: 'unknown' | 'pending' | 'ready' | 'not_installed' | 'changed';
}

export function installPackageIdentity(id: unknown, version: unknown): InstallPackageIdentity | undefined {
  if (typeof id !== 'string' || typeof version !== 'string' || !/^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$/.test(id) || !/^[a-zA-Z0-9][a-zA-Z0-9._+-]{0,127}$/.test(version)) return;
  return { id, version };
}

export async function readLocalInstallIdentity(files: File[]): Promise<InstallPackageIdentity | undefined> {
  const manifests = files.filter(file => /(^|\/)plugin\.yaml$/.test((file as any).webkitRelativePath || file.name));
  manifests.sort((a, b) => ((a as any).webkitRelativePath || a.name).split('/').length - ((b as any).webkitRelativePath || b.name).split('/').length);
  const file = manifests[0];
  if (!file || file.size > 1024 * 1024) return;
  const depth = ((file as any).webkitRelativePath || file.name).split('/').length;
  if (manifests[1] && ((manifests[1] as any).webkitRelativePath || manifests[1].name).split('/').length === depth) return;
  const text = await file.text();
  const field = (key: string) => {
    const match = text.match(new RegExp(`^${key}:[ \\t]*(?:"([^"\\r\\n]+)"|'([^'\\r\\n]+)'|([^ \\t#\\r\\n]+))[ \\t]*(?:#.*)?\\r?$`, 'm'));
    return match?.[1] || match?.[2] || match?.[3];
  };
  return installPackageIdentity(field('id'), field('version'));
}

// 写入结果未知时只接受目标版本及本次提交的修订号，不能把同版本旧安装当成成功。
export function reconcileInstall(state: InstallRecovery, inspection: InstallInspection): InstallRecovery {
  const next = { ...state, inspection, outcome: 'unknown' as InstallRecovery['outcome'] };
  if (!state.target || inspection.plugin_id !== state.target.id) return next;
  const current = inspection.installation;
  if (current.reason === 'PLUGIN_NOT_INSTALLED' && !current.version) return { ...next, outcome: 'not_installed' };
  if (!current.version || !current.revision) return next;
  if (current.version !== state.target.version || (state.receiptRevision && current.revision !== state.receiptRevision)) return { ...next, outcome: 'changed' };
  if (!state.receiptRevision && state.force && state.baselineRevision === current.revision) return next;
  const ready = !state.enable || (current.ready === true && current.tenant_ready === true && current.runtime_ready === true);
  return { ...next, outcome: ready ? 'ready' : 'pending' };
}

export function canResumeInstall(state: InstallRecovery): boolean {
  return state.enable && state.outcome === 'pending' && !!state.target && !!state.inspection?.installation.revision;
}

export function uncertainInstallError(error: any): boolean {
  const status = Number(error?.status || error?.statusCode || error?.response?.status || error?.cause?.status || 0);
  const message = String(error?.message || '').toLowerCase();
  return status >= 500 || error?.name === 'AbortError' || error?.cause?.name === 'AbortError' || error?.cause?.name === 'TimeoutError' || /timeout|aborted|请求超时|网络错误|network error/.test(message);
}
