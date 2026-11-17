<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { HostSnapshot } from '~/composables/api/services/hostMonitorService';
import { hostBytes } from './hostPresentation';
import { interfaceKind, isAssignedConnection, usableAddresses, type HostNetworkInterface } from './networkPresentation';
const props = defineProps<{ network: HostSnapshot['network'] }>();
const { locale } = useI18n();
const showDetails = ref(false);
const zh = computed(() => locale.value.startsWith('zh'));
const text = computed(() => zh.value ? {
  title: '网络连接', subtitle: '先看已启用、已分配 IP 的连接；内部接口按需展开。',
  assigned: '已分配 IP 的连接', noAssigned: '没有发现已启用且分配了可用 IP 的连接。可展开全部接口检查。',
  ip: 'IP 地址', received: '累计接收', sent: '累计发送', noCounters: '未采集到', noAddress: '未分配 IP',
  up: '已启用', down: '未启用', unknown: '状态未上报', local: '本机内部通信', tunnel: '隧道接口', virtual: '系统 / 虚拟接口', card: '网卡',
  note: '接收指这台设备收到的数据，发送指这台设备发出的数据。这里是网卡计数起点以来的累计量，不是当前网速；有 IP 也不等于已验证互联网连通。',
  details: '查看全部接口', hide: '收起接口详情', why: '回环用于本机程序通信；隧道用于系统或 VPN 等连接。没有 IP、未启用的接口不代表网络故障。',
  name: '接口', type: '用途', state: '启用状态', addresses: '全部地址', empty: '未报告网络接口。', partial: '部分接口的计数未采集到；已采集的数据仍可查看。',
} : {
  title: 'Network connections', subtitle: 'Enabled connections with assigned IPs first; internal interfaces are available in details.',
  assigned: 'Connections with assigned IPs', noAssigned: 'No enabled connection with a usable assigned IP was reported. Open all interfaces to inspect.',
  ip: 'IP address', received: 'Received total', sent: 'Sent total', noCounters: 'Not collected', noAddress: 'No assigned IP',
  up: 'Enabled', down: 'Disabled', unknown: 'State not reported', local: 'Local communication', tunnel: 'Tunnel', virtual: 'System / virtual', card: 'Interface',
  note: 'Received means data arriving at this device; sent means data leaving it. Totals are since the interface counter reset, not current transfer rates. An assigned IP is not an Internet connectivity check.',
  details: 'Show all interfaces', hide: 'Hide interface details', why: 'Loopback carries local traffic. Tunnels support system or VPN connections. Disabled interfaces or missing IPs do not by themselves indicate a network fault.',
  name: 'Interface', type: 'Purpose', state: 'State', addresses: 'All addresses', empty: 'No interfaces reported.', partial: 'Some counters were not collected. Available measurements are shown.',
});
const active = computed(() => props.network.interfaces.filter(isAssignedConnection).sort((a, b) => Number(interfaceKind(a) !== 'interface') - Number(interfaceKind(b) !== 'interface')));
const bytes = (value: number | null) => hostBytes(value, text.value.noCounters);
function state(item: HostNetworkInterface) { return item.is_up === undefined ? text.value.unknown : item.is_up ? text.value.up : text.value.down; }
function kind(item: HostNetworkInterface) { return ({ loopback: text.value.local, tunnel: text.value.tunnel, virtual: text.value.virtual, interface: text.value.card })[interfaceKind(item)]; }
</script>
<template>
  <div class="min-w-0 space-y-4" data-testid="host-network">
    <div><h2 class="text-lg font-semibold">{{ text.title }}</h2><p class="mt-1 text-sm text-muted">{{ text.subtitle }}</p></div>
    <p v-if="network.status === 'partial'" class="text-sm text-muted">{{ text.partial }}</p>
    <p class="text-sm font-medium">{{ text.assigned }} · {{ active.length }}</p>
    <p v-if="!active.length" class="rounded-lg border border-default p-4 text-sm text-muted">{{ text.noAssigned }}</p>
    <div v-else class="grid min-w-0 gap-3 lg:grid-cols-2">
      <UCard v-for="item in active" :key="item.name" data-testid="host-network-connection">
        <div class="flex flex-wrap items-center justify-between gap-2"><h3 class="font-semibold">{{ text.card }} {{ item.name }}</h3><span class="text-xs text-muted">{{ kind(item) }} · {{ state(item) }}</span></div>
        <dl class="mt-4 space-y-3 text-sm">
          <div><dt class="text-muted">{{ text.ip }}</dt><dd v-for="address in usableAddresses(item)" :key="address" class="break-all font-mono">{{ address.split('/')[0] }}</dd></div>
          <div class="grid grid-cols-2 gap-3"><div><dt class="text-muted">{{ text.received }}</dt><dd class="mt-1 font-semibold tabular-nums">{{ bytes(item.rx_bytes) }}</dd></div><div><dt class="text-muted">{{ text.sent }}</dt><dd class="mt-1 font-semibold tabular-nums">{{ bytes(item.tx_bytes) }}</dd></div></div>
        </dl>
      </UCard>
    </div>
    <p class="text-sm text-muted">{{ text.note }}</p>
    <UButton color="neutral" variant="outline" :aria-expanded="showDetails" data-testid="host-network-details" @click="showDetails = !showDetails">{{ showDetails ? text.hide : text.details }} ({{ network.interfaces.length }})</UButton>
    <div v-if="showDetails" data-testid="host-network-all" class="min-w-0 space-y-3">
      <p class="text-sm text-muted">{{ text.why }}</p><p v-if="!network.interfaces.length">{{ text.empty }}</p>
      <div v-else class="overflow-x-auto rounded-lg border border-default"><table class="w-full text-left text-sm"><thead><tr><th>{{ text.name }}</th><th>{{ text.type }}</th><th>{{ text.state }}</th><th>{{ text.addresses }}</th><th>{{ text.received }}</th><th>{{ text.sent }}</th></tr></thead><tbody>
        <tr v-for="item in network.interfaces" :key="item.name"><td>{{ item.name }}</td><td>{{ kind(item) }}</td><td>{{ state(item) }}</td><td><span v-for="address in item.addresses" :key="address" class="block break-all">{{ address }}</span><span v-if="!item.addresses.length">{{ text.noAddress }}</span></td><td>{{ bytes(item.rx_bytes) }}</td><td>{{ bytes(item.tx_bytes) }}</td></tr>
      </tbody></table></div>
    </div>
  </div>
</template>
<style scoped>th, td { padding: 0.75rem; border-bottom: 1px solid var(--ui-border); } th { white-space: nowrap; }</style>
