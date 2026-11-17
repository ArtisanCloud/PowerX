import { beforeEach, expect, it, vi } from 'vitest';
import { hostFixture } from './host-fixture';
const transport = vi.hoisted(() => ({ get: vi.fn(), unwrap: vi.fn((response: any) => response.data) }));
vi.mock('~/composables/api/index', () => ({ useApiClient: () => transport }));
import { useHostMonitorService } from '~/composables/api/services/hostMonitorService';
import { hostBytes, hostPercent, hostMessages, hostWarning } from '~/components/monitor/host/hostPresentation';
beforeEach(() => vi.clearAllMocks());
it('uses standard envelope and abortable GET without transport retries', async () => {
  const snapshot = hostFixture(); transport.get.mockResolvedValue({ code: 200, data: snapshot });
  const signal = new AbortController().signal;
  expect(await useHostMonitorService().getSnapshot(signal)).toEqual(snapshot);
  expect(transport.get).toHaveBeenCalledWith('/admin/monitor/host', { signal, retry: 0, useGlobalLoading: false, useGlobalError: false });
  expect(transport.unwrap).toHaveBeenCalledOnce();
});
it('rejects malformed snapshots instead of replacing retained data', async () => {
  transport.get.mockResolvedValue({ code: 200, data: { collected_at: 'invalid' } });
  await expect(useHostMonitorService().getSnapshot()).rejects.toThrow('HOST_MONITOR_RESPONSE_INVALID');
});
it('distinguishes unavailable metrics from real zero values', () => {
  expect(hostBytes(null, 'Unavailable')).toBe('Unavailable'); expect(hostBytes(0, 'Unavailable')).toBe('0 B');
  expect(hostPercent(null, 'Unavailable')).toBe('Unavailable'); expect(hostPercent(0, 'Unavailable')).toBe('0.0%');
  expect(hostPercent(NaN, 'Unavailable')).toBe('Unavailable');
});

it('localizes collector diagnostics and safely summarizes unknown messages', () => {
  expect(hostWarning('NVIDIA monitoring unsupported: nvidia-smi unavailable', hostMessages.zh)).toBe(hostMessages.zh.gpuUnsupported);
  expect(hostWarning('CPU sample unavailable or canceled', hostMessages.zh)).toBe(hostMessages.zh.cpuWarning);
  expect(hostWarning('network interfaces unavailable', hostMessages.en)).toBe(hostMessages.en.networkWarning);
  expect(hostWarning('unexpected internal diagnostic', hostMessages.zh)).toBe(hostMessages.zh.warningUnknown);
});
