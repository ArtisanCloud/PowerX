import type { HostSnapshot } from '~/composables/api/services/hostMonitorService';
export type HostDisk = HostSnapshot['disks'][number];
// Classify mounts, not hypervisors: a VM's real root/data disks remain monitored.
export function diskDetailReason(disk: HostDisk): 'image' | 'internal' | 'readonly' | null {
  const path = disk.mount_point.replace(/\/+$/, '') || '/';
  if (/^\/Library\/Developer\/CoreSimulator\/(Volumes|Cryptex)(\/|$)/.test(path) || ['iso9660', 'udf', 'squashfs'].includes(disk.filesystem.toLowerCase())) return 'image';
  if (['tmpfs', 'devtmpfs', 'proc', 'sysfs', 'devfs', 'ramfs'].includes(disk.filesystem.toLowerCase()) || /^\/System\/Volumes\/(Preboot|Update|Recovery|VM|iSCPreboot|Hardware|xarts)(\/|$)/.test(path) || /^\/(private\/)?var\/vm(\/|$)/.test(path)) return 'internal';
  // Keep the OS root visible even if its system volume is read-only; no capacity alarm.
  if (disk.read_only === true && path !== '/') return 'readonly';
  return null;
}
export function isMainDisk(disk: HostDisk): boolean { return diskDetailReason(disk) === null; }
export function canAlertDisk(disk: HostDisk): boolean { return isMainDisk(disk) && disk.read_only !== true; }
