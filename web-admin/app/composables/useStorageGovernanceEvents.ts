import { onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue';
import { connectBackupEvents } from '~/utils/backup-sse';

// 事件只触发变更与重连后的权威读取；不以周期查询替代状态推送。
export function useStorageGovernanceEvents(enabled: Ref<boolean>, open: (signal: AbortSignal) => Promise<ReadableStream<Uint8Array>>, change: () => void, identity?: Ref<string>) {
  const status = ref<'idle' | 'connecting' | 'connected' | 'disconnected'>('idle');
  let stop: (() => void) | undefined, mounted = false, connectedBefore = false;
  const connect = () => {
    stop?.(); stop = undefined;
    if (!mounted || !enabled.value) { status.value = 'idle'; return; }
    stop = connectBackupEvents(open, event => { if (event.name === 'change') change(); }, value => {
      status.value = value;
      if (value === 'connected') { if (connectedBefore) change(); connectedBefore = true; }
    });
  };
  onMounted(() => { mounted = true; connect(); });
  watch([enabled, () => identity?.value], () => { connectedBefore = false; connect(); });
  onBeforeUnmount(() => { mounted = false; stop?.(); stop = undefined; });
  return { status };
}
