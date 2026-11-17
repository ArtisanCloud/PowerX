import type { HostSnapshot } from '~/composables/api/services/hostMonitorService';
export function hostFixture(): HostSnapshot {
  return {
    collected_at: '2026-11-16T08:00:00Z', duration_ms: 205, hostname: 'fixture-host', uptime_seconds: 86400,
    os: 'linux', arch: 'amd64', scope: 'host', scope_note: 'OS-visible resources', container_detected: false,
    cpu: { status: 'ok', logical_cores: 4, usage_percent: 23.5, sample_ms: 200 },
    memory: { status: 'ok', total_bytes: 8192, used_bytes: 5120, available_bytes: 3072 },
    swap: { status: 'ok', total_bytes: 0, used_bytes: 0, available_bytes: 0 },
    disks: [
      { status: 'ok', mount_point: '/run', filesystem: 'tmpfs', total_bytes: 100, used_bytes: 100, available_bytes: 0 },
      { status: 'ok', mount_point: '/', filesystem: 'ext4', total_bytes: 110, used_bytes: 95, available_bytes: 5 },
    ],
    network: { status: 'ok', interfaces: [{ name: 'eth0', is_up: true, is_loopback: false, point_to_point: false, addresses: ['192.0.2.1/24'], rx_bytes: 1024, tx_bytes: null }] },
    gpu: { status: 'unsupported', devices: [] }, warnings: ['NVIDIA monitoring unsupported: nvidia-smi unavailable'],
  };
}
