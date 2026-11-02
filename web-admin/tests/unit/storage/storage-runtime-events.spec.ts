import { describe, it, expect, vi } from 'vitest';
import { defineComponent, ref, nextTick } from 'vue';
import { mount } from '@vue/test-utils';
import { useStorageGovernanceEvents } from '~/composables/useStorageGovernanceEvents';
const streams = vi.hoisted(() => ({ calls: [] as any[], closed: vi.fn() }));
vi.mock('~/utils/backup-sse', () => ({ connectBackupEvents: (open: any, receive: any, state: any) => { streams.calls.push({ open, receive, state }); return streams.closed; } }));
describe('storage event identity lifecycle', () => {
  it('reconnects for another Root identity and releases connections on revoke and unmount', async () => {
    streams.calls = []; streams.closed.mockClear(); const enabled = ref(true), identity = ref('root-a'), changed = vi.fn();
    const w = mount(defineComponent({ setup() { useStorageGovernanceEvents(enabled, vi.fn(), changed, identity); return {}; }, template: '<div />' }));
    expect(streams.calls).toHaveLength(1); streams.calls[0].state('connected'); streams.calls[0].receive({ name: 'change' }); expect(changed).toHaveBeenCalledTimes(1);
    identity.value = 'root-b'; await nextTick(); expect(streams.closed).toHaveBeenCalledTimes(1); expect(streams.calls).toHaveLength(2);
    enabled.value = false; await nextTick(); expect(streams.closed).toHaveBeenCalledTimes(2); expect(streams.calls).toHaveLength(2);
    enabled.value = true; await nextTick(); expect(streams.calls).toHaveLength(3); w.unmount(); expect(streams.closed).toHaveBeenCalledTimes(3);
  });
});
