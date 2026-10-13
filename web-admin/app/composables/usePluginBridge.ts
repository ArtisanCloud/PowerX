import { effectScope, watch } from 'vue';
import { useI18n, useColorMode } from '#imports';
import { PluginBridgeHost, type FrameStatus } from '~/utils/plugin-bridge-host';

let host: PluginBridgeHost | null = null;
let scope: ReturnType<typeof effectScope> | null = null;

export function usePluginBridge() {
  const { locale } = useI18n();
  const colorMode = useColorMode();
  const auth = useAuth();
  if (process.client && !host) {
    host = new PluginBridgeHost(() => ({
      locale: String(locale.value),
      theme: colorMode.preference === 'auto' ? 'system' : colorMode.preference,
    }), 8000, auth.refreshAuth);
    scope = effectScope(true);
    scope.run(() => watch([locale, () => colorMode.preference], () => host?.broadcastMetadata()));
  }
  return {
    prepareFrame: () => auth.initAuth(),
    register: (frame: HTMLIFrameElement, meta: { pluginId: string; instanceId: string; onStatus: (status: FrameStatus) => void }) =>
      host?.register(frame, meta.pluginId, meta.instanceId, meta.onStatus),
    unregister: (frame?: HTMLIFrameElement | null) => { if (frame) host?.unregister(frame); },
    navigateFrame: (frame: HTMLIFrameElement, path: string) => host?.navigate(frame, path),
  };
}

if (import.meta.hot) import.meta.hot.dispose(() => { host?.dispose(); scope?.stop(); host = null; scope = null; });
