import { afterEach, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { onMounted, ref } from 'vue';
import App from '~/app.vue';

afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });

it('does not overwrite an existing operation message or release its locks after app mount', async () => {
  vi.useFakeTimers();
  const locks = ref(2), manual = ref(true), message = ref('正在保存操作');
  vi.stubGlobal('useHead', vi.fn());
  vi.stubGlobal('onMounted', onMounted);
  vi.stubGlobal('useGL_LockCount', () => locks);
  vi.stubGlobal('useGL_ManualVisible', () => manual);
  vi.stubGlobal('useGlobalLoading', () => ({
    show: ({ message: text }: { message: string }) => { manual.value = true; message.value = text; },
    hide: () => { manual.value = false; },
    setMessage: (text: string) => { message.value = text; },
    setProgress: vi.fn(),
  }));
  const wrapper = mount(App, { global: { stubs: {
    UApp: { template: '<div><slot/></div>' }, NuxtLayout: { template: '<main><slot/></main>' },
    NuxtPage: true, NuxtLoadingIndicator: true, ClientOnly: true, GlobalAlertNotification: true,
  } } });
  try {
    await vi.advanceTimersByTimeAsync(1500);
    expect(locks.value).toBe(2);
    expect(manual.value).toBe(true);
    expect(message.value).toBe('正在保存操作');
  } finally { wrapper.unmount(); }
});
