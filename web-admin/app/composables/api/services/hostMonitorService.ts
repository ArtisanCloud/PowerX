import { useApiClient } from '../index';
import type { ApiResponse } from '../types/types';

export type HostMetricStatus = 'ok' | 'partial' | 'unsupported' | 'error';
export interface HostCapacity {
  status: HostMetricStatus;
  total_bytes: number | null;
  available_bytes: number | null;
  used_bytes: number | null;
}
export interface HostSnapshot {
  collected_at: string;
  duration_ms: number;
  hostname: string;
  uptime_seconds: number | null;
  os: string;
  arch: string;
  scope: 'host' | 'container_visible' | 'unknown';
  container_detected: boolean;
  scope_note: string;
  cpu: { status: HostMetricStatus; logical_cores: number; usage_percent: number | null; sample_ms: number };
  memory: HostCapacity;
  swap: HostCapacity;
  disks: (HostCapacity & { mount_point: string; filesystem: string; read_only?: boolean | null })[];
  network: { status: HostMetricStatus; interfaces: { name: string; is_up?: boolean; is_loopback?: boolean; point_to_point?: boolean; addresses: string[]; rx_bytes: number | null; tx_bytes: number | null }[] };
  gpu: { status: HostMetricStatus; devices: { index: number; name: string; utilization_percent: number | null; memory_total_bytes: number | null; memory_used_bytes: number | null; temperature_celsius: number | null }[] };
  warnings: string[];
}

export const useHostMonitorService = () => {
  const api = useApiClient();
  return {
    async getSnapshot(signal?: AbortSignal): Promise<HostSnapshot> {
      const response = await api.get<ApiResponse<HostSnapshot>>('/admin/monitor/host', {
        signal, retry: 0, useGlobalLoading: false, useGlobalError: false,
      });
      const snapshot = api.unwrap<HostSnapshot>(response);
      if (!snapshot || !Number.isFinite(Date.parse(snapshot.collected_at)) || !snapshot.cpu || !snapshot.memory ||
        !snapshot.swap || !Array.isArray(snapshot.disks) || !Array.isArray(snapshot.network?.interfaces) ||
        !Array.isArray(snapshot.gpu?.devices) || !Array.isArray(snapshot.warnings)) {
        throw new Error('HOST_MONITOR_RESPONSE_INVALID');
      }
      return snapshot;
    },
  };
};
