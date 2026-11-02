// 小文件不能因统一四舍五入到 GiB 而显示成零；未知值保留为未知。
export function formatStorageBytes(value?: number | null): string {
  if (value == null || !Number.isSafeInteger(value) || value < 0) return '—';
  const unit = value >= 1024 ** 3 ? 'GiB' : value >= 1024 ** 2 ? 'MiB' : value >= 1024 ? 'KiB' : 'B';
  const divisor = unit === 'GiB' ? 1024 ** 3 : unit === 'MiB' ? 1024 ** 2 : unit === 'KiB' ? 1024 : 1;
  return `${(value / divisor).toLocaleString(undefined, { maximumFractionDigits: 2 })} ${unit}`;
}
