import type { CustomerImportResult } from '~/composables/api/services/customerService';

export function customerNameValid(value: string, required = false): boolean {
  const text = value.trim(), characters = Array.from(text);
  return (!required || text.length > 0) && characters.length <= 128 && !/[\u0000-\u001f\u007f-\u009f]/u.test(text) && !characters.some(c => { const point = c.codePointAt(0)!; return point >= 0xd800 && point <= 0xdfff; });
}

export function customerAdminError(error: any, fallback: string): string {
  const data = error?.data || error?.response?._data;
  return [...new Set([data?.message || error?.message || fallback, data?.error_code || data?.error, data?.trace_id || data?.request_id].filter(Boolean))].join(' · ');
}

export const CUSTOMER_IMPORT_TEMPLATE = 'type,display_name,primary_email,primary_phone,primary_contact_name,primary_contact_email,primary_contact_phone\nperson,Example Person,person@example.com,,,,\ncompany,Example Company,company@example.com,,Example Contact,contact@example.com,\n';

export function customerImportResultCSV(result: CustomerImportResult): string {
  // 导出仅包含系统行号/状态/错误码/UUID；不将用户姓名等文字输出为电子表格公式。
  const rows = [['row', 'status', 'error_code', 'customer_uuid', 'primary_contact_uuid'], ...result.rows.map(row => [String(row.row), row.status, row.error_code || '', row.customer_uuid || '', row.primary_contact_uuid || ''])];
  return '\ufeff' + rows.map(row => row.map(value => `"${value.replaceAll('"', '""')}"`).join(',')).join('\r\n');
}

export function downloadCustomerCSV(name: string, content: string) {
  const url = URL.createObjectURL(new Blob([content], { type: 'text/csv;charset=utf-8' }));
  const a = document.createElement('a'); a.href = url; a.download = name; a.click();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}
