import type { HostSnapshot } from '~/composables/api/services/hostMonitorService';
export type HostNetworkInterface = HostSnapshot['network']['interfaces'][number];
export function interfaceKind(item: HostNetworkInterface): 'loopback' | 'tunnel' | 'virtual' | 'interface' {
  if (item.is_loopback || /^lo\d*$/.test(item.name)) return 'loopback';
  if (item.point_to_point || /^(utun|tun|tap|wg|gif|stf|ipsec|tailscale)/.test(item.name)) return 'tunnel';
  if (/^(awdl|llw|anpi|veth|docker|br-|bridge)/.test(item.name)) return 'virtual';
  return 'interface';
}
export function usableAddresses(item: HostNetworkInterface): string[] {
  return item.addresses.filter(address => {
    const ip = address.split('/')[0]!.split('%')[0]!.toLowerCase();
    return ip && ip !== '::1' && ip !== '::' && ip !== '0.0.0.0' && !ip.startsWith('127.') && !ip.startsWith('169.254.') && !/^fe[89ab]/.test(ip);
  }).sort((a, b) => Number(b.includes('.')) - Number(a.includes('.')));
}
export function isAssignedConnection(item: HostNetworkInterface): boolean {
  return item.is_up !== false && interfaceKind(item) !== 'loopback' && usableAddresses(item).length > 0;
}
