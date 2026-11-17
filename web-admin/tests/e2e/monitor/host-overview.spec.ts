import { test, expect } from '@playwright/test';

const GiB = 1024 ** 3;
const snapshot = () => ({
  collected_at: new Date().toISOString(), duration_ms: 205, os: 'linux', arch: 'amd64', hostname: 'dashboard-fixture', uptime_seconds: 86400,
  scope: 'container_visible', container_detected: true, scope_note: 'OS-visible resources; not container limits',
  cpu: { status: 'ok', logical_cores: 4, usage_percent: 23.5, sample_ms: 200 },
  memory: { status: 'ok', total_bytes: 8 * GiB, available_bytes: 3 * GiB, used_bytes: 5 * GiB },
  swap: { status: 'ok', total_bytes: 0, available_bytes: 0, used_bytes: 0 },
  disks: [{ status: 'ok', mount_point: '/', filesystem: 'ext4', total_bytes: 100 * GiB, available_bytes: 5 * GiB, used_bytes: 95 * GiB }, { status: 'ok', mount_point: '/Library/Developer/CoreSimulator/Volumes/iOS_21C62', filesystem: 'apfs', read_only: true, total_bytes: 20 * GiB, available_bytes: GiB, used_bytes: 19 * GiB }],
  network: { status: 'ok', interfaces: [{ name: 'eth0', is_up: true, is_loopback: false, addresses: ['192.0.2.1/24'], rx_bytes: 2 * GiB, tx_bytes: GiB }, { name: 'lo0', is_up: true, is_loopback: true, addresses: ['127.0.0.1/8'], rx_bytes: 50 * GiB, tx_bytes: 50 * GiB }, { name: 'gif0', is_up: false, point_to_point: true, addresses: [], rx_bytes: 0, tx_bytes: 0 }] },
  gpu: { status: 'unsupported', devices: [] }, warnings: ['NVIDIA monitoring unsupported: nvidia-smi unavailable'],
});

for (const width of [1440, 390, 320]) {
  test(`host snapshot manual refresh and retained navigation at ${width}px`, async ({ browser, baseURL }, info) => {
    const context = await browser.newContext({ viewport: { width, height: 950 }, storageState: { cookies: [], origins: [] } });
    const page = await context.newPage();
    const errors: string[] = [];
    page.on('pageerror', e => errors.push(e.message));
    const tenant = '00000000-0000-4000-8000-000000000001';
    await context.addInitScript(tenant => {
      const expiresAt = Date.now() + 3600000;
      const token = `header.${btoa(JSON.stringify({ exp: Math.floor(expiresAt / 1000), tid: tenant }))}.fixture`;
      localStorage.setItem('powerx.session', JSON.stringify({ version: 1, status: 'active', sessionId: 'host-overview-proof', revision: 1, accessToken: token, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: tenant }));
      localStorage.setItem('welcome_guide_completed', 'true'); localStorage.setItem('px_locale', 'zh');
    }, tenant);
    let requests = 0, failing = false;
    await page.routeWebSocket(/\/api\/ws/, socket => socket.close());
    await page.route(url => url.pathname.startsWith('/api/'), async route => {
      const path = new URL(route.request().url()).pathname;
      if (path.includes('_nuxt_icon')) return route.fallback();
      let data: any = {};
      if (path.endsWith('/setup/status')) data = { configured: true, requires_login: true, install_status: 'installed' };
      else if (path.endsWith('/me/context')) data = { is_root: true, current_tenant_uuid: tenant, current_member_id: 1, user: { id: 1, display_name: '主机概览验收', status: 1, is_root: true }, members: [{ tenant_uuid: tenant, tenant_name: '验收', member_id: 1, is_admin: true }] };
      else if (path.endsWith('/menus')) data = { categories: [{ id: 'monitor', title: '监控中心', order: 1, children: [{ id: 'host', title: '主机概览', path: '/monitor/host', order: 1, visible: true, origin: 'system' }] }] };
      else if (path.endsWith('/monitor/host')) {
        requests++;
        if (failing) return route.fulfill({ status: 503, json: { code: 503, message: 'host snapshot unavailable' } });
        data = snapshot();
      }
      return route.fulfill({ json: { code: 200, data } });
    });
    await page.goto(`${baseURL}/monitor/host`);
    await expect(page.getByTestId('host-overview')).toBeVisible();
    if (width < 768) {
      const collapse = page.getByRole('button', { name: '收起', exact: true }).first();
      if (await collapse.isVisible()) await collapse.click();
    }
    await expect.poll(() => requests).toBe(1);
    await expect(page.getByTestId('host-panel')).toContainText('23.5');
    for (const tab of ['compute', 'disks', 'network', 'gpu', 'overview']) {
      await page.getByTestId(`host-tab-${tab}`).click();
      await expect(page.getByTestId('host-panel')).toHaveAttribute('data-section', tab);
      await expect(page.getByTestId(`host-tab-${tab}`)).toHaveAttribute('aria-selected', 'true');
      if (tab === 'disks') {
        await expect(page.getByTestId('host-panel')).not.toContainText('iOS_21C62');
        await expect(page.getByTestId('host-disk-warning')).not.toContainText('CoreSimulator');
        await page.getByTestId('host-all-mounts').click();
        await expect(page.getByTestId('host-panel')).toContainText('iOS_21C62');
        await page.getByTestId('host-all-mounts').click();
      }
    }
    await page.waitForTimeout(1200);
    expect(requests).toBe(1);
    await page.getByTestId('host-tab-network').click();
    await expect(page.getByTestId('host-tab-network')).toHaveAttribute('aria-selected', 'true');
    await expect(page.getByTestId('host-panel')).toContainText('eth0');
    await expect(page.getByTestId('host-network-connection')).toHaveCount(1);
    await expect(page.getByTestId('host-network-all')).toHaveCount(0);
    await page.getByTestId('host-network-details').click();
    await expect(page.getByTestId('host-network-all')).toContainText('lo0');
    await expect(page.getByTestId('host-network-all')).toContainText('gif0');
    await expect(page.getByTestId('host-tab-network')).toHaveAttribute('aria-selected', 'true');
    expect(requests).toBe(1);
    await page.screenshot({ path: info.outputPath(`network-${width}.png`), fullPage: true });
    await page.getByTestId('host-network-details').click();
    await page.getByTestId('host-refresh').click();
    await expect.poll(() => requests).toBe(2);
    await expect(page.getByTestId('host-refresh')).toBeEnabled();
    await expect(page.getByTestId('host-panel')).toHaveAttribute('data-section', 'network');
    failing = true;
    await page.getByTestId('host-refresh').click();
    await expect(page.getByTestId('host-error')).toBeVisible();
    await expect(page.getByTestId('host-panel')).toContainText('eth0');
    await expect(page.getByTestId('host-stale')).toBeVisible();
    await expect(page.getByTestId('host-refresh')).toBeEnabled();
    failing = false;
    await page.getByTestId('host-refresh').click();
    await expect.poll(() => requests).toBe(4);
    await expect(page.getByTestId('host-error')).toHaveCount(0);
    await page.getByTestId('host-tab-overview').click();
    await expect(page.getByTestId('host-panel')).toContainText('23.5');
    await expect(page.getByTestId('host-tab-overview')).toHaveAttribute('aria-selected', 'true');
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)).toBe(true);
    expect(errors).toEqual([]);
    await page.screenshot({ path: info.outputPath(`host-${width}.png`), fullPage: true });
    await context.close();
  });
}
