import { onBeforeUnmount, onMounted, ref } from "vue";
const listeners = new Map<string, Set<() => void>>();
export function useBackupEvents(resource?: string, change?: () => void) {
  onMounted(() => {
    if (!resource || !change) return;
    if (!listeners.has(resource)) listeners.set(resource, new Set());
    listeners.get(resource)!.add(change);
  });
  onBeforeUnmount(() => { if (resource && change) listeners.get(resource)?.delete(change); });
  return { status: ref("connected") };
}
export function emitBackupEvent(resource: string) { listeners.get(resource)?.forEach(change => change()); }
