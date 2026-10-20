import { onBeforeUnmount, onMounted, readonly, ref } from "vue";
import { useApiClient } from "~/composables/api";
import { connectBackupEvents } from "~/utils/backup-sse";
import { AUTH_SESSION_EVENT, readAuthSession } from "~/utils/auth-session";

export type BackupResource = "snapshots" | "recovery" | "archives" | "restore_copies";
const handlers = new Map<BackupResource, Set<() => void>>();
const status = ref<"idle" | "connecting" | "connected" | "disconnected">("idle");
let disconnect: (() => void) | undefined;
let removeIdentityListener: (()=>void) | undefined;
const identity = () => { const session=readAuthSession(); return `${session?.sessionId || ''}:${session?.tenantUuid || ''}:${session?.status || ''}`; };

// 同一页面共用一条 Root SSE；每个分区仅按实际变更刷新，卸载最后订阅时关闭。
export function useBackupEvents(resource?: BackupResource, change?: () => void) {
  const api = useApiClient();
  const subscribe = () => {
    if (!resource || !change) return;
    if (!handlers.has(resource)) handlers.set(resource, new Set());
    handlers.get(resource)!.add(change);
    const connect = () => { if (!disconnect) disconnect = connectBackupEvents(
      signal => api.stream("/admin/ops/backup/events", signal),
      event => {
        if (event.name !== "invalidate" || !Array.isArray(event.data?.resources)) return;
        for (const name of event.data.resources) handlers.get(name)?.forEach(callback => callback());
      },
      value => { status.value = value; },
    ); };
    connect();
    if (!removeIdentityListener && typeof window!=='undefined') {
      let owner=identity();
      const changed=()=>{
        const next=identity();if(next===owner)return;owner=next;
        disconnect?.();disconnect=undefined;status.value='idle';
        if(readAuthSession()?.status==='active') {
          connect();for(const callbacks of handlers.values())for(const callback of callbacks)callback();
        }
      };
      window.addEventListener(AUTH_SESSION_EVENT,changed);window.addEventListener('storage',changed);
      removeIdentityListener=()=>{window.removeEventListener(AUTH_SESSION_EVENT,changed);window.removeEventListener('storage',changed);};
    }
  };
  onMounted(subscribe);
  onBeforeUnmount(() => {
    if (resource && change) {
      handlers.get(resource)?.delete(change);
      if (!handlers.get(resource)?.size) handlers.delete(resource);
    }
    if (!handlers.size) { disconnect?.(); disconnect = undefined; removeIdentityListener?.(); removeIdentityListener=undefined;status.value = "idle"; }
  });
  return { status: readonly(status) };
}
