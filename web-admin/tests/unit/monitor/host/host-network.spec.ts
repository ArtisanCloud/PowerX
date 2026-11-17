import { describe, expect, it, vi } from 'vitest';
import { defineComponent } from 'vue';
import { mount } from '@vue/test-utils';
import Panel from '~/components/monitor/host/HostNetworkPanel.vue';
import { interfaceKind, isAssignedConnection, usableAddresses } from '~/components/monitor/host/networkPresentation';
vi.mock('vue-i18n', async () => { const { ref } = await import('vue'); return { useI18n: () => ({ locale: ref('zh') }) }; });
const eth = { name: 'eth0', is_up: true, addresses: ['fe80::123%eth0/64', '192.0.2.1/24'], rx_bytes: 1024, tx_bytes: 2048 };
const loop = { name: 'lo0', is_up: true, is_loopback: true, addresses: ['127.0.0.1/8', '::1/128'], rx_bytes: 9999, tx_bytes: 9999 };
const down = { name: 'gif0', is_up: false, addresses: [], rx_bytes: 0, tx_bytes: 0 };
describe('network task presentation', () => {
  it('prioritizes assigned IPs without claiming Internet connectivity or showing internal interfaces as faults', () => {
    expect(isAssignedConnection(eth)).toBe(true);
    expect(usableAddresses(eth)).toEqual(['192.0.2.1/24']);
    expect(isAssignedConnection(loop)).toBe(false);
    expect(isAssignedConnection(down)).toBe(false);
    expect(isAssignedConnection({ ...eth, is_up: false })).toBe(false);
    expect(interfaceKind(loop)).toBe('loopback');
    expect(interfaceKind(down)).toBe('tunnel');
    expect(usableAddresses({ ...eth, addresses: ['169.254.1.2/16', 'fe80::1/64'] })).toEqual([]);
  });
  it('keeps loopback and unused interfaces in explicitly opened details; uses understandable byte labels', async () => {
    const Card = defineComponent({ template: '<div><slot /></div>' });
    const Button = defineComponent({ template: '<button><slot /></button>' });
    const wrapper = mount(Panel, { props: { network: { status: 'ok', interfaces: [loop, down, eth] } }, global: { stubs: { UCard: Card, UButton: Button } } });
    expect(wrapper.findAll('[data-testid=host-network-connection]')).toHaveLength(1);
    expect(wrapper.text()).toContain('网卡 eth0');
    expect(wrapper.text()).toContain('累计接收');
    expect(wrapper.text()).toContain('累计发送');
    expect(wrapper.text()).not.toContain('lo0');
    expect(wrapper.text()).not.toContain('gif0');
    await wrapper.get('[data-testid=host-network-details]').trigger('click');
    expect(wrapper.get('[data-testid=host-network-all]').text()).toContain('lo0');
    expect(wrapper.text()).toContain('本机内部通信');
    expect(wrapper.text()).toContain('未分配 IP');
    wrapper.unmount();
  });
});
