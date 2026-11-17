import { describe, expect, it } from 'vitest';
import { canAlertDisk, diskDetailReason, isMainDisk, type HostDisk } from '~/components/monitor/host/diskPresentation';
const disk = (mount_point: string, filesystem = 'apfs', read_only?: boolean): HostDisk => ({ mount_point, filesystem, read_only, status: 'ok', total_bytes: 100, used_bytes: 99, available_bytes: 1 });
describe('capacity alert mount scope', () => {
  it('keeps simulator images and OS internal mounts in details even with old backend metadata', () => {
    for (const path of ['/Library/Developer/CoreSimulator/Volumes/iOS_21C62', '/Library/Developer/CoreSimulator/Volumes/xrOS_21N305', '/Library/Developer/CoreSimulator/Cryptex/Images/bundle']) {
      expect(isMainDisk(disk(path))).toBe(false);
      expect(canAlertDisk(disk(path))).toBe(false);
      expect(diskDetailReason(disk(path))).toBe('image');
    }
    expect(isMainDisk(disk('/System/Volumes/Preboot'))).toBe(false);
    expect(isMainDisk(disk('/run', 'tmpfs'))).toBe(false);
    expect(canAlertDisk(disk('/Volumes/Installer', 'iso9660'))).toBe(false);
    expect(canAlertDisk(disk('/Volumes/ReadOnly', 'apfs', true))).toBe(false);
  });
  it('keeps real system/data/external disks, including a VM guest root and VM storage data volume', () => {
    for (const d of [disk('/', 'ext4', false), disk('/data', 'ext4', false), disk('/System/Volumes/Data'), disk('/Volumes/External'), disk('/var/lib/libvirt/images', 'ext4', false)]) {
      expect(isMainDisk(d)).toBe(true);
      expect(canAlertDisk(d)).toBe(true);
    }
    expect(isMainDisk(disk('/', 'apfs', true))).toBe(true);
    expect(canAlertDisk(disk('/', 'apfs', true))).toBe(false);
  });
});
