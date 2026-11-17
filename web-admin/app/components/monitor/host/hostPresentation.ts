export const hostMessages = {
  en: {
    scopeNote: 'Resources visible to the operating system; these are not container limits.',
    warningUnknown: 'Some host metrics could not be collected.',
    memoryWarning: 'Memory or swap metrics are unavailable.', cpuWarning: 'CPU sampling is unavailable or was canceled.',
    mountsWarning: 'Mount information is unavailable.', remoteWarning: 'Space metrics for remote or userspace mounts are unsupported.',
    diskWarning: 'Some filesystem space metrics are unavailable.', platformWarning: 'This platform collector is unsupported.',
    networkWarning: 'Network interfaces are unavailable.', networkPartialWarning: 'Some network addresses or cumulative counters are unavailable.',
    gpuErrorWarning: 'NVIDIA collection failed or timed out.', gpuPartialWarning: 'NVIDIA devices were not reported or metrics are incomplete.',
    storageGovernance: 'Open storage governance', title: 'Host overview', refresh: 'Refresh', rootOnly: 'Host monitoring is available to Root users only.',
    overview: 'Overview', compute: 'CPU / Memory', disks: 'Disks', network: 'Network', gpu: 'GPU',
    sampled: 'Last sampled', never: 'Not sampled', loading: 'Collecting host metrics…',
    failed: 'Unable to refresh host metrics', stale: 'Stale data — refresh failed. The last successful sample is retained.',
    empty: 'No sample available. Refresh to try again.', warnings: 'Collection warnings',
    cpu: 'CPU', memory: 'Memory', swap: 'Swap', used: 'Used', available: 'Available', total: 'Total',
    cores: 'Logical cores', sample: 'CPU sample window', duration: 'Collection duration',
    hostname: 'Hostname', uptime: 'Uptime', platform: 'Platform', scope: 'Observation scope',
    host: 'Host', container_visible: 'Container-visible resources', unknown: 'Unknown', container: 'Container detected',
    yes: 'Yes', no: 'No', unavailable: 'Unavailable', ok: 'Available', partial: 'Partially available',
    unsupported: 'Unsupported', error: 'Collection failed', mount: 'Mount point', filesystem: 'Filesystem',
    interface: 'Interface', addresses: 'Addresses', rx: 'Received (cumulative)', tx: 'Sent (cumulative)',
    counters: 'Counters are cumulative bytes, not transfer rates.', devices: 'Reported devices',
    gpuUnsupported: 'NVIDIA monitoring is unavailable. This does not indicate the absence of a GPU.',
    noDevices: 'No devices were reported by this collector.', noDisks: 'No filesystems were reported.',
    noInterfaces: 'No network interfaces were reported.', utilization: 'Utilization', temperature: 'Temperature',
    manual: 'Loads on entry; use Refresh for a new sample.', days: 'days', hours: 'hours',
  },
  zh: {
    scopeNote: '显示操作系统可见资源，不代表容器资源限额。',
    warningUnknown: '部分主机指标未能完成采集。',
    memoryWarning: '内存或交换空间指标不可用。', cpuWarning: 'CPU 采样不可用或已取消。',
    mountsWarning: '挂载信息不可用。', remoteWarning: '暂不支持远程或用户态挂载的空间采集。',
    diskWarning: '部分文件系统空间指标不可用。', platformWarning: '暂不支持当前平台的指标采集。',
    networkWarning: '网络接口信息不可用。', networkPartialWarning: '部分网络地址或累计流量计数不可用。',
    gpuErrorWarning: 'NVIDIA 指标采集失败或超时。', gpuPartialWarning: '未报告 NVIDIA 设备，或设备指标不完整。',
    storageGovernance: '查看存储治理', title: '主机概览', refresh: '手动刷新', rootOnly: '仅 Root 用户可查看主机监控。',
    overview: '概览', compute: '计算 / 内存', disks: '磁盘', network: '网络', gpu: 'GPU',
    sampled: '最后采样', never: '尚未采样', loading: '正在采集主机指标…',
    failed: '主机指标刷新失败', stale: '数据已过期 — 刷新失败，保留上次成功采样。',
    empty: '暂无采样数据，请手动刷新重试。', warnings: '采集警告',
    cpu: 'CPU', memory: '内存 Memory', swap: '交换空间 Swap', used: '已用', available: '可用', total: '总量',
    cores: '逻辑核心', sample: 'CPU 采样窗口', duration: '采集耗时',
    hostname: '主机名', uptime: '运行时长', platform: '系统架构', scope: '观测范围',
    host: '主机', container_visible: '容器可见资源', unknown: '未知', container: '检测到容器',
    yes: '是', no: '否', unavailable: '不可用', ok: '可用', partial: '部分可用',
    unsupported: '不支持', error: '采集失败', mount: '挂载点', filesystem: '文件系统',
    interface: '网络接口', addresses: '地址', rx: '累计接收', tx: '累计发送',
    counters: '此处为累计字节数，不是实时传输速率。', devices: '已报告设备',
    gpuUnsupported: 'NVIDIA 采集器不可用，不代表主机没有 GPU。',
    noDevices: '采集器未报告设备。', noDisks: '未报告文件系统。',
    noInterfaces: '未报告网络接口。', utilization: '利用率', temperature: '温度',
    manual: '进入时自动采样，后续通过手动刷新更新。', days: '天', hours: '小时',
  },
};
export function hostBytes(value: number | null | undefined, unavailable: string): string {
  if (value == null || !Number.isFinite(value) || value < 0) return unavailable;
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB'];
  const index = value === 0 ? 0 : Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / 1024 ** index).toFixed(index ? 1 : 0)} ${units[index]}`;
}
export function hostPercent(value: number | null | undefined, unavailable: string): string {
  return value == null || !Number.isFinite(value) || value < 0 ? unavailable : `${value.toFixed(1)}%`;
}

const warningKeys = {
  'memory or swap metrics unavailable': 'memoryWarning',
  'CPU sample unavailable or canceled': 'cpuWarning',
  'CPU sample unavailable': 'cpuWarning',
  'mount information unavailable': 'mountsWarning',
  'remote or userspace mount space unsupported': 'remoteWarning',
  'mount space metrics unavailable': 'diskWarning',
  'platform collector unsupported': 'platformWarning',
  'network interfaces unavailable': 'networkWarning',
  'some network addresses or byte counters unavailable': 'networkPartialWarning',
  'NVIDIA monitoring unsupported: nvidia-smi unavailable': 'gpuUnsupported',
  'NVIDIA query failed or timed out': 'gpuErrorWarning',
  'NVIDIA devices absent or metrics incomplete': 'gpuPartialWarning',
} as const;
export function hostWarning(message: string, text: typeof hostMessages.en | typeof hostMessages.zh): string {
  const key = warningKeys[message as keyof typeof warningKeys];
  return key ? text[key] : text.warningUnknown;
}
