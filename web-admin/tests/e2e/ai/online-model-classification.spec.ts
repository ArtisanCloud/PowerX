import { test, expect } from '@playwright/test';

for (const narrow of [false, true]) {
  test(`image online model provenance and selector (${narrow ? 'narrow' : 'desktop'})`, async ({ page }, info) => {
    await page.setViewportSize(narrow ? { width: 390, height: 844 } : { width: 1440, height: 1000 });
    const tenant = '00000000-0000-4000-8000-000000000001';
    await page.addInitScript(tenant => {
      const expiresAt = Date.now() + 3600_000;
      const token = `header.${btoa(JSON.stringify({ exp: Math.floor(expiresAt / 1000), tid: tenant }))}.fixture`;
      localStorage.setItem('powerx.session', JSON.stringify({ version: 1, status: 'active', sessionId: 'image-model-proof', revision: 1, accessToken: token, refreshToken: 'fixture', tokenType: 'Bearer', expiresAt, scope: 'access', tenantUuid: tenant }));
      localStorage.setItem('welcome_guide_completed', 'true'); localStorage.setItem('px_locale', 'zh');
    }, tenant);
    const syncRequests: any[] = [], writes: string[] = [], errors: string[] = [];
    let synced = false;
    const snapshot = { provider: 'openai', env: 'dev', app: '', protocol: 'openai', source: 'https://openai.example.invalid/v1/models', updated_at: '2026-10-30T00:00:00Z', revision: 'fixed-revision', corrected_count: 3, models: [
      { id: 'gpt-image-2', label: 'GPT Image 2', modalities: ['image'], manual_modalities: [], match: 'local' },
      { id: 'chatgpt-image-latest', label: 'ChatGPT Image', modalities: ['image'], manual_modalities: [], match: 'local' },
      { id: 'gpt-4o', label: 'GPT 4o', modalities: ['llm'], manual_modalities: [], match: 'local' },
      { id: 'legacy-unknown', label: 'Legacy unknown', modalities: [], manual_modalities: [], review_required: true, match: 'unmatched' },
      { id: 'reported-audio', label: 'Reported audio', modalities: [], manual_modalities: [], reported_modalities: ['audio'], match: 'unmatched' },
    ] };
    const profile = (modality: string) => ({ env: 'dev', modality, provider: 'openai', model: modality === 'image' ? 'gpt-image-1' : 'legacy-unknown', defaults: {} });
    page.on('pageerror', error => errors.push(error.message));
    await page.routeWebSocket(/\/api\/ws/, socket => socket.close());
    await page.route(url => url.pathname.startsWith('/api/'), async route => {
      const url = new URL(route.request().url()), path = url.pathname;
      if (path.includes('/_nuxt_icon/')) return route.fallback();
      let data: unknown = {};
      if (path.endsWith('/setup/status')) data = { configured: true, requires_login: true, install_status: 'installed' };
      else if (path.endsWith('/me/context')) data = { is_root: true, current_tenant_uuid: tenant, current_member_id: 1, user: { id: 1, display_name: '分类验收', status: 1, is_root: true }, members: [{ tenant_uuid: tenant, tenant_name: '验收', member_id: 1, is_admin: true }] };
      else if (path.endsWith('/menus')) data = { menus: [{ id: 'ai', title: '模型配置', path: '/settings/ai', visible: true, origin: 'system' }] };
      else if (path.endsWith('/agents/providers')) data = { providers: [{ id: 'openai', name: 'OpenAI', configured: true, auth: { scheme: 'bearer', fields: ['api_key', 'base_url'], defaults: { base_url: 'https://openai.example.invalid/v1' } } }] };
      else if (path.endsWith('/agents/settings/profiles')) data = { env: 'dev', profiles: [profile('llm'), profile('image')] };
      else if (path.endsWith('/agents/settings/credentials')) data = { env: 'dev', credentials: [{ provider: 'openai', data: { base_url: 'https://openai.example.invalid/v1' } }] };
      else if (path.endsWith('/agents/settings/active')) data = { profile: profile(url.searchParams.get('modality') || 'llm') };
      else if (path.endsWith('/agents/models/sync')) { synced = true; syncRequests.push(route.request().postDataJSON()); data = snapshot; }
      else if (path.endsWith('/agents/models')) data = { models: url.searchParams.get('modality') === 'image' ? ['gpt-image-1', ...(synced ? ['gpt-image-2', 'chatgpt-image-latest'] : [])] : ['gpt-4o'] };
      else if (path.endsWith('/skills/source-policy')) data = { allowlist: ['builtin', 'plugin'], effective_source: 'default' };
      else if (route.request().method() !== 'GET') writes.push(path);
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ code: 200, data }) });
    });
    await page.goto('/settings/ai?modality=image');
    const model = page.getByTestId('ai-model-select');
    await expect(model).toContainText('gpt-image-1');
    if (narrow) {
      const backdrop = page.locator('[class~="fixed"][class~="inset-0"][class~="z-40"][class~="md:hidden"]');
      if (await backdrop.isVisible()) await backdrop.click({ position: { x: 380, y: 100 } });
    }
    await page.getByTestId('ai-online-models').click();
    const dialog = page.getByRole('dialog');
    await expect(dialog.getByTestId('online-model-summary')).toContainText('在线获取 5 个型号；供应商已声明用途 1 个，未声明 4 个');
    const list = dialog.getByTestId('online-model-list');
    await expect(list).toContainText('gpt-image-2'); await expect(list).toContainText('gpt-4o');
    await dialog.getByRole('tab', { name: '供应商已声明用途 (1)' }).click();
    await expect(list).toContainText('reported-audio'); await expect(list).not.toContainText('gpt-image-2');
    await dialog.getByRole('tab', { name: '用途未声明 (4)' }).click();
    await expect(list).toContainText('legacy-unknown'); await expect(list).toContainText('gpt-image-2');
    await expect(dialog.getByTestId('online-model-assign')).toHaveCount(0);
    await dialog.getByRole('tab', { name: '全部在线型号 (5)' }).click();
    await dialog.getByPlaceholder('搜索型号或名称').fill('gpt-image-2');
    await dialog.getByRole('tab', { name: '供应商已声明用途 (1)' }).click();
    await dialog.getByRole('tab', { name: '全部在线型号 (5)' }).click();
    await expect(dialog.getByPlaceholder('搜索型号或名称')).toHaveValue('gpt-image-2');
    await expect(list).toContainText('gpt-image-2');
    const bounds = await dialog.boundingBox();
    expect(bounds!.x).toBeGreaterThanOrEqual(0); expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(narrow ? 390 : 1440);
    expect(await dialog.evaluate(element => element.scrollWidth - element.clientWidth)).toBeLessThanOrEqual(1);
    await page.screenshot({ path: info.outputPath(`image-classification-${narrow ? 'narrow' : 'desktop'}.png`), fullPage: true, animations: 'disabled' });
    await dialog.getByRole('button', { name: '完成', exact: true }).click();
    await expect(model).toContainText('gpt-image-1');
    await model.click();
    await expect(page.getByRole('option', { name: 'gpt-image-2', exact: true })).toBeVisible();
    await page.getByRole('option', { name: 'gpt-image-2', exact: true }).click();
    await expect(model).toContainText('gpt-image-2');
    expect(syncRequests).toEqual([{ env: 'dev', provider: 'openai', app: '', modality: 'image', action: 'refresh' }]);
    await page.getByRole('button', { name: 'LLM 文本', exact: true }).click();
    await expect(model).toContainText('legacy-unknown');
    await expect(page.getByTestId('ai-saved-model-review')).toContainText('已保留当前配置');
    expect(writes).toEqual([]); expect(errors).toEqual([]);
  });
}
